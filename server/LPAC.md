# lpac Integration

SMS Hub uses the external [lpac](https://github.com/estkme-group/lpac) executable as its GSMA SGP.22 Local Profile Assistant. The Go API forwards lpac's APDU operations over MQTT to the ESP32 terminal; lpac performs ES9+ HTTPS, ES10b authentication, Bound Profile Package download, installation, cancellation, and confirmation-code handling.

## License

lpac is a multi-license project. The official CLI and stdio driver used by this integration are distributed under AGPL-3.0-only or the vendor's commercial license. SMS Hub does not vendor or modify lpac source code. The Docker build fetches the pinned upstream release and preserves its license files in the build source. Operators distributing an image or offering a modified lpac over a network must comply with the applicable lpac license. See upstream `REUSE.toml` and `LICENSES/` for file-level terms.

## Platform support

Profile download is supported only when the Go API runs on Linux, including the provided Docker deployment. Windows builds of the API explicitly reject new download tasks because the upstream MinGW lpac `stdio` backend does not reliably wait for remote MQTT APDU responses.

Windows can still run the management API and all non-download features: Profile listing, enable, disable, delete, SMS, diagnostics, and subscriptions. Run the API in Docker/Linux when Profile download is required.

## Linux and Docker

`docker compose build api` compiles the pinned lpac release and installs it at `/usr/local/bin/lpac`. Run the stack normally with `docker compose up`.

For a native Linux API process, install lpac from an upstream release and either put it on `PATH` or set:

```bash
export LPAC_PATH=/opt/lpac/lpac
```

The executable must include the `stdio` APDU backend and `curl` HTTP backend.

## Network and security requirements

- The API host must reach public SM-DP+ HTTPS endpoints with a valid system CA store and correct clock.
- MQTT must stay connected throughout the download. One APDU session per terminal is allowed.
- The activation code is passed directly to lpac and is only masked in audit records. It is not included in terminal logs.
- Use authenticated MQTT and TLS outside trusted LANs. The default unauthenticated broker configuration is unsuitable for Internet exposure because APDU access controls the eUICC.
- A profile activation code may be single-use. Test first with a disposable profile and do not interrupt power during installation.

The implementation follows the flow supported by lpac (SGP.22 v2.2.2 compatibility). Interoperability with later optional SGP.22 features depends on upstream lpac and the target eUICC/SM-DP+.
