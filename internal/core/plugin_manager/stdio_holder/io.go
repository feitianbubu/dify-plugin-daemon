package stdio_holder

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/langgenius/dify-plugin-daemon/internal/types/entities/plugin_entities"
	"github.com/langgenius/dify-plugin-daemon/internal/utils/log"
	"github.com/langgenius/dify-plugin-daemon/internal/utils/parser"
)

var (
	stdio_holder sync.Map                        = sync.Map{}
	l            *sync.Mutex                     = &sync.Mutex{}
	listeners    map[string]func(string, []byte) = map[string]func(string, []byte){}
)

type stdioHolder struct {
	id              string
	plugin_identity string
	writer          io.WriteCloser
	reader          io.ReadCloser
	err_reader      io.ReadCloser
	l               *sync.Mutex
	listener        map[string]func([]byte)
	error_listener  map[string]func([]byte)
	started         bool

	err_message                 string
	last_err_message_updated_at time.Time

	health_chan        chan bool
	health_chan_closed bool
	health_chan_lock   *sync.Mutex
	last_active_at     time.Time
}

func (s *stdioHolder) Error() error {
	if time.Since(s.last_err_message_updated_at) < 60*time.Second {
		if s.err_message != "" {
			return errors.New(s.err_message)
		}
	}

	return nil
}

func (s *stdioHolder) Stop() {
	s.writer.Close()
	s.reader.Close()
	s.err_reader.Close()

	s.health_chan_lock.Lock()
	if !s.health_chan_closed {
		close(s.health_chan)
		s.health_chan_closed = true
	}
	s.health_chan_lock.Unlock()

	stdio_holder.Delete(s.id)
}

func (s *stdioHolder) StartStdout() {
	s.started = true
	defer s.Stop()

	scanner := bufio.NewScanner(s.reader)
	for scanner.Scan() {
		data := scanner.Bytes()
		if len(data) == 0 {
			continue
		}

		event, err := parser.UnmarshalJsonBytes[plugin_entities.PluginUniversalEvent](data)
		if err != nil {
			// log.Error("unmarshal json failed: %s", err.Error())
			continue
		}

		session_id := event.SessionId

		switch event.Event {
		case plugin_entities.PLUGIN_EVENT_LOG:
			if event.Event == plugin_entities.PLUGIN_EVENT_LOG {
				logEvent, err := parser.UnmarshalJsonBytes[plugin_entities.PluginLogEvent](event.Data)
				if err != nil {
					log.Error("unmarshal json failed: %s", err.Error())
					continue
				}

				log.Info("plugin %s: %s", s.plugin_identity, logEvent.Message)
			}
		case plugin_entities.PLUGIN_EVENT_SESSION:
			for _, listener := range listeners {
				listener(s.id, event.Data)
			}

			for listener_session_id, listener := range s.listener {
				if listener_session_id == session_id {
					listener(event.Data)
				}
			}
		case plugin_entities.PLUGIN_EVENT_ERROR:
			log.Error("plugin %s: %s", s.plugin_identity, event.Data)
		case plugin_entities.PLUGIN_EVENT_HEARTBEAT:
			s.last_active_at = time.Now()
		}
	}
}

func (s *stdioHolder) WriteError(msg string) {
	const MAX_ERR_MSG_LEN = 1024
	reduce := len(msg) + len(s.err_message) - MAX_ERR_MSG_LEN
	if reduce > 0 {
		s.err_message = s.err_message[reduce:]
	}

	s.err_message += msg
	s.last_err_message_updated_at = time.Now()
}

func (s *stdioHolder) StartStderr() {
	for {
		buf := make([]byte, 1024)
		n, err := s.err_reader.Read(buf)
		if err != nil && err != io.EOF {
			break
		} else if err != nil {
			s.WriteError(fmt.Sprintf("%s\n", buf[:n]))
			break
		}

		if n > 0 {
			s.WriteError(fmt.Sprintf("%s\n", buf[:n]))
		}
	}
}

func (s *stdioHolder) Wait() error {
	s.health_chan_lock.Lock()
	if s.health_chan_closed {
		s.health_chan_lock.Unlock()
		return errors.New("you need to start the health check before waiting")
	}
	s.health_chan_lock.Unlock()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// check status of plugin every 5 seconds
	for {
		s.health_chan_lock.Lock()
		if s.health_chan_closed {
			s.health_chan_lock.Unlock()
			break
		}
		s.health_chan_lock.Unlock()
		select {
		case <-ticker.C:
			// check heartbeat
			if time.Since(s.last_active_at) > 20*time.Second {
				return errors.New("plugin is not active")
			}
		case <-s.health_chan:
			// closed
			return s.Error()
		}
	}

	return nil
}

func (s *stdioHolder) GetID() string {
	return s.id
}
