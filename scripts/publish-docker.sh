#!/usr/bin/env bash
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REMOTE="${REMOTE:-origin}"
BRANCH="${BRANCH:-$(git -C "$ROOT" symbolic-ref --quiet --short HEAD || true)}"
IMAGE="${IMAGE:-xmoli/sms-relay}"
GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"
PUSH_RETRIES="${PUSH_RETRIES:-3}"
NO_CACHE="${NO_CACHE:-0}"
VERSION_TAG="${1:-}"
CONTAINER="sms-relay-publish-$$"
VOLUME="sms-relay-publish-data-$$"

cleanup() {
  docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
  docker volume rm "$VOLUME" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

log() {
  printf '\n==> %s\n' "$*"
}

fail() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

for command in git docker curl timeout; do
  command -v "$command" >/dev/null 2>&1 || fail "required command not found: $command"
done

[[ -n "$BRANCH" ]] || fail "detached HEAD is not supported; check out the release branch first"
[[ "$PUSH_RETRIES" =~ ^[1-9][0-9]*$ ]] || fail "PUSH_RETRIES must be a positive integer"
if [[ -n "$VERSION_TAG" && ! "$VERSION_TAG" =~ ^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$ ]]; then
  fail "invalid Docker tag: $VERSION_TAG"
fi

cd "$ROOT"
[[ -z "$(git status --porcelain)" ]] || fail "working tree is not clean; commit or stash changes before publishing"
docker info >/dev/null

log "Fetching $REMOTE/$BRANCH"
git fetch "$REMOTE" "$BRANCH"
git merge --ff-only "$REMOTE/$BRANCH"

read -r behind ahead < <(git rev-list --left-right --count "$REMOTE/$BRANCH...HEAD")
[[ "$behind" == "0" ]] || fail "local branch is behind $REMOTE/$BRANCH after merge"
[[ "$ahead" == "0" ]] || fail "local branch has $ahead unpushed commit(s); push Git before publishing the image"
[[ -z "$(git status --porcelain)" ]] || fail "working tree changed during synchronization"

commit="$(git rev-parse HEAD)"
log "Building $IMAGE:latest from ${commit:0:12}"
build_args=(
  --tag "$IMAGE:latest"
  --file "$ROOT/server/api/Dockerfile"
  --build-arg "GOPROXY=$GOPROXY"
)
if [[ "$NO_CACHE" == "1" ]]; then
  build_args+=(--no-cache)
fi
docker build "${build_args[@]}" "$ROOT/server"

if [[ -n "$VERSION_TAG" && "$VERSION_TAG" != "latest" ]]; then
  docker tag "$IMAGE:latest" "$IMAGE:$VERSION_TAG"
fi

log "Running image smoke tests"
docker volume create "$VOLUME" >/dev/null
docker run -d --name "$CONTAINER" \
  -p 127.0.0.1::8080 \
  -p 127.0.0.1::1883 \
  -v "$VOLUME:/data" \
  "$IMAGE:latest" >/dev/null

http_port="$(docker port "$CONTAINER" 8080/tcp | head -n 1 | awk -F: '{print $NF}')"
mqtt_port="$(docker port "$CONTAINER" 1883/tcp | head -n 1 | awk -F: '{print $NF}')"
[[ -n "$http_port" && -n "$mqtt_port" ]] || fail "failed to resolve published smoke-test ports"

healthy=0
for _ in $(seq 1 30); do
  if curl -fsS "http://127.0.0.1:$http_port/api/health" | grep -q '"status":"ok"'; then
    healthy=1
    break
  fi
  sleep 1
done
[[ "$healthy" == "1" ]] || {
  docker logs "$CONTAINER" >&2 || true
  fail "API health check failed"
}

curl -fsS -o /dev/null "http://127.0.0.1:$http_port/"
curl -fsS -o /dev/null "http://127.0.0.1:$http_port/devices"
timeout 3 bash -c "</dev/tcp/127.0.0.1/$mqtt_port" || fail "MQTT port is not reachable"
docker exec "$CONTAINER" sh -c \
  'test "$(awk "/^Uid:/ {print \$2}" /proc/1/status)" = 10001 && test "$(stat -c %u /data/smshub.db)" = 10001 && test -x /usr/local/bin/lpac'
if docker exec "$CONTAINER" ldd /usr/local/bin/lpac | grep -q 'not found'; then
  fail "lpac has missing runtime dependencies"
fi

cleanup
trap - EXIT INT TERM

registry_status="$(curl -4 -sS -o /dev/null -w '%{http_code}' --connect-timeout 5 --max-time 15 https://registry-1.docker.io/v2/ || true)"
if [[ "$registry_status" != "401" ]]; then
  printf 'warning: Docker Registry IPv4 preflight returned HTTP %s; push will still be attempted\n' "${registry_status:-unreachable}" >&2
fi

tags=(latest)
if [[ -n "$VERSION_TAG" && "$VERSION_TAG" != "latest" ]]; then
  tags+=("$VERSION_TAG")
fi

for tag in "${tags[@]}"; do
  log "Pushing $IMAGE:$tag"
  pushed=0
  for attempt in $(seq 1 "$PUSH_RETRIES"); do
    if docker push "$IMAGE:$tag"; then
      pushed=1
      break
    fi
    printf 'push attempt %d/%d failed; retrying in 5 seconds\n' "$attempt" "$PUSH_RETRIES" >&2
    sleep 5
  done
  [[ "$pushed" == "1" ]] || fail "failed to push $IMAGE:$tag after $PUSH_RETRIES attempts"

  expected_id="$(docker image inspect "$IMAGE:$tag" --format '{{.Id}}')"
  docker pull "$IMAGE:$tag" >/dev/null
  remote_id="$(docker image inspect "$IMAGE:$tag" --format '{{.Id}}')"
  [[ "$remote_id" == "$expected_id" ]] || fail "remote verification failed for $IMAGE:$tag"
done

log "Published successfully"
printf 'image:  %s\n' "$IMAGE"
printf 'tags:   %s\n' "${tags[*]}"
printf 'commit: %s\n' "$commit"
printf 'digest: %s\n' "$(docker image inspect "$IMAGE:latest" --format '{{json .RepoDigests}}')"
