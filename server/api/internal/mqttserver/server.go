package mqttserver

import (
	"context"
	"log"
	"strings"

	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
	"github.com/mochi-mqtt/server/v2/packets"

	"sms-forwarding/server/api/internal/store"
)

type Server struct {
	broker *mqtt.Server
	addr   string
	store  *store.Store
}

type logHook struct {
	mqtt.HookBase
	store *store.Store
}

func (h *logHook) ID() string {
	return "sms-hub-log"
}

func (h *logHook) Provides(b byte) bool {
	return b == mqtt.OnSessionEstablished || b == mqtt.OnDisconnect || b == mqtt.OnPublish || b == mqtt.OnPacketRead
}

func (h *logHook) OnSessionEstablished(cl *mqtt.Client, _ packets.Packet) {
	log.Printf("mqtt client connected id=%s", cl.ID)
	if h.store != nil && strings.HasPrefix(cl.ID, "esp32-") {
		if err := h.store.MarkTerminalOnline(cl.ID); err != nil {
			log.Printf("mqtt connect online mark failed device=%s: %v", cl.ID, err)
		}
	}
}

func (h *logHook) OnPacketRead(cl *mqtt.Client, pk packets.Packet) (packets.Packet, error) {
	if h.store != nil && strings.HasPrefix(cl.ID, "esp32-") {
		if err := h.store.MarkTerminalOnline(cl.ID); err != nil {
			log.Printf("mqtt packet online mark failed device=%s: %v", cl.ID, err)
		}
	}
	return pk, nil
}

func (h *logHook) OnDisconnect(cl *mqtt.Client, err error, _ bool) {
	if err != nil {
		log.Printf("mqtt client disconnected id=%s err=%v", cl.ID, err)
	} else {
		log.Printf("mqtt client disconnected id=%s", cl.ID)
	}
	if h.store != nil && strings.HasPrefix(cl.ID, "esp32-") {
		if markErr := h.store.MarkTerminalOffline(cl.ID); markErr != nil {
			log.Printf("mqtt disconnect offline mark failed device=%s: %v", cl.ID, markErr)
		}
	}
}

func (h *logHook) OnPublish(cl *mqtt.Client, pk packets.Packet) (packets.Packet, error) {
	clientID := ""
	if cl != nil {
		clientID = cl.ID
	}
	log.Printf("mqtt publish client=%s topic=%s bytes=%d", clientID, pk.TopicName, len(pk.Payload))
	return pk, nil
}

func New(addr string, store *store.Store) *Server {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		addr = ":1883"
	}
	return &Server{addr: addr, store: store}
}

func (s *Server) Start(ctx context.Context) error {
	broker := mqtt.New(nil)
	if err := broker.AddHook(new(auth.AllowHook), nil); err != nil {
		return err
	}
	if err := broker.AddHook(&logHook{store: s.store}, nil); err != nil {
		return err
	}
	if err := broker.AddListener(listeners.NewTCP(listeners.Config{ID: "tcp", Address: s.addr})); err != nil {
		return err
	}
	s.broker = broker

	go func() {
		<-ctx.Done()
		if err := broker.Close(); err != nil {
			log.Printf("embedded mqtt broker close failed: %v", err)
		}
	}()

	go func() {
		log.Printf("embedded mqtt broker listening on %s", s.addr)
		if err := broker.Serve(); err != nil && ctx.Err() == nil {
			log.Printf("embedded mqtt broker stopped: %v", err)
		}
	}()
	return nil
}
