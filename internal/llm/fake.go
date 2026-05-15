package llm

import (
	"context"
	"errors"
	"sync"
)

// FakeClient is a deterministic LLMClient driven by a URL-keyed table.
// Used by judge unit tests and the e2e fake-LLM dry run that powers
// goal criterion F.
type FakeClient struct {
	mu        sync.Mutex
	responses map[string]*JudgeResponse
	defaults  *JudgeResponse
}

// NewFakeClient builds a FakeClient.
func NewFakeClient() *FakeClient {
	return &FakeClient{responses: map[string]*JudgeResponse{}}
}

// SetResponse maps pageURL -> response.
func (f *FakeClient) SetResponse(pageURL string, r JudgeResponse) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rc := r
	f.responses[pageURL] = &rc
}

// SetDefault sets the response returned when pageURL isn't in the map.
func (f *FakeClient) SetDefault(r JudgeResponse) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rc := r
	f.defaults = &rc
}

// Judge returns the configured response for req.PageURL.
func (f *FakeClient) Judge(_ context.Context, req JudgeRequest) (*JudgeResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r, ok := f.responses[req.PageURL]; ok {
		clone := *r
		return &clone, nil
	}
	if f.defaults != nil {
		clone := *f.defaults
		return &clone, nil
	}
	return nil, errors.New("fake llm: no response configured for " + req.PageURL)
}
