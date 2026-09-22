package credentials

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestNewLiveGitHubAPIIsFailClosedWithoutAuthorization(t *testing.T) {
	api, err := NewLiveGitHubAPI(LiveAuthorization{})
	if !errors.Is(err, ErrLiveUnauthorized) || api != nil {
		t.Fatalf("unauthorized live GitHub adapter was constructed: api=%v err=%v", api, err)
	}
}

func TestPrepareForegroundCannotUseUnauthorizedLiveAdapter(t *testing.T) {
	api, err := NewLiveGitHubAPI(LiveAuthorization{})
	if !errors.Is(err, ErrLiveUnauthorized) || api != nil {
		t.Fatalf("live adapter constructor did not fail closed: api=%v err=%v", api, err)
	}
	session, err := PrepareForeground(context.Background(), fixtureDocument(), fixtureSource(t), api, nil)
	if !errors.Is(err, ErrConfig) || !reflect.DeepEqual(session, ForegroundSession{}) {
		t.Fatalf("nil live adapter crossed the foreground boundary: session=%+v err=%v", session, err)
	}
}
