package livecanary

import (
	"context"
	"errors"
	"testing"

	"github.com/actions/scaleset"
)

type invalidOwnedAPI struct {
	API
	fault string
}

func (a invalidOwnedAPI) GetScaleSet(ctx context.Context, id int) (*scaleset.RunnerScaleSet, error) {
	set, err := a.API.GetScaleSet(ctx, id)
	if err != nil || set == nil {
		return set, err
	}
	copy := *set
	switch a.fault {
	case "id":
		copy.ID = 99
	case "name":
		copy.Name = "unexpected-scale-set"
	case "group":
		copy.RunnerGroupID = 99
	case "missing label":
		copy.Labels = nil
	case "different label":
		copy.Labels = []scaleset.Label{{Name: "unexpected-label"}}
	case "nil object":
		return nil, nil
	}
	return &copy, nil
}

func TestInvalidOwnedProofCannotBeClearedBeforeFirstCleanup(t *testing.T) {
	for _, phase := range []string{"inspect", "jit-loss", "before-ack"} {
		for _, fault := range []string{"id", "name", "group", "missing label", "different label", "nil object"} {
			for _, durability := range []string{"fresh driver", "file reopen"} {
				t.Run(phase+"/"+fault+"/"+durability, func(t *testing.T) {
					d, f, memory := created(t)
					var baseAPI API = f
					var file *FileJournal
					var directory string
					if durability == "file reopen" {
						baseAPI = statisticsInventoryAPI{f}
						directory = privateDir(t)
						var err error
						file, err = openTestJournal(t, directory, d.Approval)
						if err != nil {
							t.Fatal("private owned-proof journal")
						}
						defer func() { _ = file.Close() }()
						for _, e := range memory.Events() {
							if e.Kind == "inventory" {
								e.Digest = fixtureInventory
							}
							if file.Append(e) != nil {
								t.Fatal("private owned-proof receipt")
							}
						}
						d.Journal = file
					}
					d.API = invalidOwnedAPI{baseAPI, fault}
					if err := d.Run(context.Background(), phase); !errors.Is(err, ErrQuarantine) || f.jitCalls != 0 || f.session.ack != 0 || f.session.acquire != 0 || f.session.close != 0 {
						t.Fatalf("invalid owned proof allowed a phase: %v", err)
					}
					if file != nil {
						if file.Close() != nil {
							t.Fatal("close invalid-proof fixture")
						}
						var err error
						file, err = openTestJournal(t, directory, d.Approval)
						if err != nil {
							t.Fatal("reopen invalid-proof fixture")
						}
						d.Journal = file
					}
					restarted := Driver{d.Approval, d.Journal, baseAPI}
					if err := restarted.Run(context.Background(), "inspect"); err != nil {
						t.Fatalf("later matching zero inspection refused: %v", err)
					}
					err := restarted.Run(context.Background(), "cleanup") // First cleanup attempt.
					if !errors.Is(err, ErrQuarantine) || f.deleteCalls != 0 || !replay(d.Journal.Events()).uncertain {
						t.Fatalf("invalid proof forgotten after matching data: cleanup=%v deletes=%d uncertain=%t", err, f.deleteCalls, replay(d.Journal.Events()).uncertain)
					}
					if replay(d.Journal.Events()).setID != 7 {
						t.Fatal("foreign response replaced original ownership receipt")
					}
				})
			}
		}
	}
}
