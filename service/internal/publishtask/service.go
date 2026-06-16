package publishtask

import (
	"fmt"
	"sync"
	"time"

	"doc-publish-server/internal/auth"
)

type TaskStatus string

const (
	StatusRunning TaskStatus = "running"
	StatusSuccess TaskStatus = "success"
	StatusFailed  TaskStatus = "failed"
)

type Task struct {
	ID        string     `json:"id"`
	Type      string     `json:"type"`
	Books     []string   `json:"books"`
	Status    TaskStatus `json:"status"`
	Logs      []string   `json:"logs"`
	ResultURL string     `json:"result_url"`
	ErrorMsg  string     `json:"error_msg"`
	Done      bool       `json:"done"`

	subscribers []chan string
}

type Service struct {
	mu    sync.RWMutex
	tasks map[string]*Task
}

func New() *Service {
	return &Service{tasks: map[string]*Task{}}
}

func (s *Service) Create(taskType string, books []string) *Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	task := &Task{
		ID:     fmt.Sprintf("%d_%s", time.Now().Unix(), auth.GenerateToken()),
		Type:   taskType,
		Books:  append([]string(nil), books...),
		Status: StatusRunning,
	}
	s.tasks[task.ID] = task
	return clone(task)
}

func (s *Service) AppendLog(taskID string, line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[taskID]
	if !ok {
		return
	}
	task.Logs = append(task.Logs, line)
	for _, sub := range task.subscribers {
		select {
		case sub <- line:
		default:
		}
	}
}

func (s *Service) Finish(taskID string, status TaskStatus, resultURL string, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[taskID]
	if !ok {
		return
	}
	task.Status = status
	task.ResultURL = resultURL
	task.ErrorMsg = errMsg
	task.Done = true
	for _, sub := range task.subscribers {
		close(sub)
	}
	task.subscribers = nil
}

func (s *Service) Get(taskID string) (*Task, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	task, ok := s.tasks[taskID]
	if !ok {
		return nil, false
	}
	return clone(task), true
}

func (s *Service) Subscribe(taskID string) (<-chan string, *Task, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[taskID]
	if !ok {
		return nil, nil, false
	}
	ch := make(chan string, 32)
	if task.Done {
		close(ch)
		return ch, clone(task), true
	}
	task.subscribers = append(task.subscribers, ch)
	return ch, clone(task), true
}

func clone(task *Task) *Task {
	cp := *task
	cp.Books = append([]string(nil), task.Books...)
	cp.Logs = append([]string(nil), task.Logs...)
	cp.subscribers = nil
	return &cp
}
