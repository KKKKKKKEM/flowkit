package sse

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/KKKKKKKEM/flowkit/core"
	"github.com/google/uuid"
)

type sseClient struct {
	ch     chan Event
	closed chan struct{}
}

type pendingAnswer struct {
	result core.InteractionResult
	err    error
}

type Session struct {
	ID        string
	Mu        sync.Mutex
	seq       int64
	buf       []Event
	trackBuf  map[string]Event // Track 事件按 tag 只保留最新一条
	client    *sseClient
	answerChs map[string]chan pendingAnswer
	Done      bool
	createdAt time.Time
}

func newSSESession(sid string) *Session {
	return &Session{
		ID:        sid,
		answerChs: make(map[string]chan pendingAnswer),
		createdAt: time.Now(),
		trackBuf:  make(map[string]Event),
	}
}

func (s *Session) Emit(eventType EventType, data any) Event {
	s.Mu.Lock()
	s.seq++
	e := Event{Seq: s.seq, Type: eventType, Data: data}

	if eventType == Track {
		// Track 事件按 tag 去重，只保留最新
		tag := extractTag(data)
		s.trackBuf[tag] = e
	} else {
		// 关键事件全量保留
		s.buf = append(s.buf, e)
	}

	client := s.client
	s.Mu.Unlock()

	if client != nil {
		select {
		case client.ch <- e:
		case <-client.closed:
		}
	}
	return e
}

func (s *Session) Subscribe(lastSeq int64) (<-chan Event, func()) {
	closed := make(chan struct{})

	s.Mu.Lock()
	if s.client != nil {
		close(s.client.closed)
	}

	var replay []Event
	for _, e := range s.trackBuf {
		if e.Seq > lastSeq {
			replay = append(replay, e)
		}
	}
	for _, e := range s.buf {
		if e.Seq > lastSeq {
			replay = append(replay, e)
		}
	}
	sort.Slice(replay, func(i, j int) bool {
		return replay[i].Seq < replay[j].Seq
	})

	// 容量 = replay 条数 + 64（留给后续实时事件）
	ch := make(chan Event, len(replay)+64)
	c := &sseClient{ch: ch, closed: closed}
	s.client = c

	// 现在 channel 容量一定够，不会阻塞
	for _, e := range replay {
		ch <- e
	}
	s.Mu.Unlock()
	return ch, func() {
		s.Mu.Lock()
		if s.client == c {
			s.client = nil
		}
		s.Mu.Unlock()
		close(closed)
	}
}

// 从 Track 事件 data 里取 tag，用于 trackBuf 的 key
func extractTag(data any) string {
	if m, ok := data.(map[string]any); ok {
		if tag, ok := m["tag"].(string); ok {
			return tag
		}
	}
	return ""
}

func (s *Session) suspend(interactionID string, i core.Interaction) (*core.InteractionResult, error) {
	ch := make(chan pendingAnswer, 1)
	s.Mu.Lock()
	s.answerChs[interactionID] = ch
	s.Mu.Unlock()

	s.Emit(Interact, InteractEventData{InteractionID: interactionID, Interaction: i})

	ans := <-ch

	s.Mu.Lock()
	delete(s.answerChs, interactionID)
	s.Mu.Unlock()

	return &ans.result, ans.err
}

func (s *Session) Answer(interactionID string, result core.InteractionResult) error {
	s.Mu.Lock()
	ch, ok := s.answerChs[interactionID]
	s.Mu.Unlock()

	if !ok {
		return fmt.Errorf("interaction %q not found or already answered", interactionID)
	}
	ch <- pendingAnswer{result: result}
	return nil
}

type SessionStore struct {
	mu       sync.Mutex
	sessions map[string]*Session
	ttl      time.Duration
	timeout  time.Duration
}

func NewSSESessionStore(ttl, timeout time.Duration) *SessionStore {
	s := &SessionStore{
		sessions: make(map[string]*Session),
		ttl:      ttl,
		timeout:  timeout,
	}
	go s.gc()
	return s
}

func DefaultSSESessionStore() *SessionStore {
	return NewSSESessionStore(30*time.Minute, 7*24*time.Hour)
}

func (s *SessionStore) GetOrCreate(sessionID string) (*Session, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sessionID != "" {
		if sess, ok := s.sessions[sessionID]; ok {
			return sess, false, nil // 已存在
		} else {
			// 前端传了 sessionID，但找不到对应 session，可能是过期了，返回错误让前端重试（新建 session）
			return nil, false, fmt.Errorf("session %q not found", sessionID)
		}
	} else {
		sessionID = uuid.NewString()
	}

	sess := newSSESession(sessionID)
	s.sessions[sessionID] = sess
	return sess, true, nil
}

func (s *SessionStore) Get(sessionID string) (*Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[sessionID]
	return sess, ok
}

func (s *SessionStore) Delete(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
}

func (s *SessionStore) gc() {
	ticker := time.NewTicker(s.ttl) // GC 频率和 TTL 解耦
	for range ticker.C {
		// 第一步：快照所有 id，立刻释放锁
		s.mu.Lock()
		ids := make([]string, 0, len(s.sessions))
		for id := range s.sessions {
			ids = append(ids, id)
		}
		s.mu.Unlock()

		// 第二步：逐个检查，不持 s.mu
		for _, id := range ids {
			s.mu.Lock()
			sess, ok := s.sessions[id]
			s.mu.Unlock()
			if !ok {
				continue
			}

			sess.Mu.Lock()
			expired := sess.Done || time.Since(sess.createdAt) > s.timeout
			sess.Mu.Unlock()

			if expired {
				s.mu.Lock()
				delete(s.sessions, id)
				s.mu.Unlock()
			}
		}
	}
}
