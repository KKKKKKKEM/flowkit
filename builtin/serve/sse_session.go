package serve

import (
	"fmt"
	"sync"
	"time"

	"github.com/KKKKKKKEM/flowkit/core"
)

type sseClient struct {
	ch     chan SSEEvent
	closed chan struct{}
}

type pendingAnswer struct {
	result core.InteractionResult
	err    error
}

type SSESession struct {
	mu        sync.Mutex
	seq       int64
	buf       []SSEEvent
	client    *sseClient
	answerChs map[string]chan pendingAnswer
	done      bool
}

func newSSESession() *SSESession {
	return &SSESession{
		answerChs: make(map[string]chan pendingAnswer),
	}
}

func (s *SSESession) emit(eventType SSEEventType, data any) SSEEvent {
	s.mu.Lock()
	s.seq++
	e := SSEEvent{Seq: s.seq, Type: eventType, Data: data}
	s.buf = append(s.buf, e)
	client := s.client
	s.mu.Unlock()

	if client != nil {
		select {
		case client.ch <- e:
		case <-client.closed:
		}
	}
	return e
}

func (s *SSESession) subscribe(lastSeq int64) (<-chan SSEEvent, func()) {
	ch := make(chan SSEEvent, 64)
	closed := make(chan struct{})
	c := &sseClient{ch: ch, closed: closed}

	s.mu.Lock()
	if s.client != nil {
		close(s.client.closed)
	}
	s.client = c
	replay := make([]SSEEvent, 0)
	for _, e := range s.buf {
		if e.Seq > lastSeq {
			replay = append(replay, e)
		}
	}
	s.mu.Unlock()

	for _, e := range replay {
		ch <- e
	}

	return ch, func() {
		s.mu.Lock()
		if s.client == c {
			s.client = nil
		}
		s.mu.Unlock()
		close(closed)
	}
}

func (s *SSESession) EmitProgress(data any) {
	s.emit(SSEProgress, data)
}

func (s *SSESession) suspend(interactionID string, i core.Interaction) (core.InteractionResult, error) {
	ch := make(chan pendingAnswer, 1)
	s.mu.Lock()
	s.answerChs[interactionID] = ch
	s.mu.Unlock()

	s.emit(SSEInteract, InteractEventData{InteractionID: interactionID, Interaction: i})

	ans := <-ch

	s.mu.Lock()
	delete(s.answerChs, interactionID)
	s.mu.Unlock()

	return ans.result, ans.err
}

func (s *SSESession) answer(interactionID string, result core.InteractionResult) error {
	s.mu.Lock()
	ch, ok := s.answerChs[interactionID]
	s.mu.Unlock()

	if !ok {
		return fmt.Errorf("interaction %q not found or already answered", interactionID)
	}
	ch <- pendingAnswer{result: result}
	return nil
}

type SSESessionStore struct {
	mu       sync.Mutex
	sessions map[string]*SSESession
	ttl      time.Duration
}

func NewSSESessionStore(ttl time.Duration) *SSESessionStore {
	s := &SSESessionStore{
		sessions: make(map[string]*SSESession),
		ttl:      ttl,
	}
	go s.gc()
	return s
}

func DefaultSSESessionStore() *SSESessionStore {
	return NewSSESessionStore(30 * time.Minute)
}

func (s *SSESessionStore) Create(sessionID string) *SSESession {
	sess := newSSESession()
	s.mu.Lock()
	s.sessions[sessionID] = sess
	s.mu.Unlock()
	return sess
}

func (s *SSESessionStore) Get(sessionID string) (*SSESession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[sessionID]
	return sess, ok
}

func (s *SSESessionStore) Delete(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
}

func (s *SSESessionStore) gc() {
	ticker := time.NewTicker(s.ttl)
	for range ticker.C {
		s.mu.Lock()
		for id, sess := range s.sessions {
			sess.mu.Lock()
			done := sess.done
			sess.mu.Unlock()
			if done {
				delete(s.sessions, id)
			}
		}
		s.mu.Unlock()
	}
}
