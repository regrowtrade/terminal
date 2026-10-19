package database

import (
	"context"
	"errors"
	"testing"
)

func TestSavePlanIntentsRejectsInvalidInputBeforeDatabase(t *testing.T) {
	store := &Store{}
	if _, err := store.SavePlanIntents(context.Background(), "synthetic", "invalid", "00000000-0000-4000-8000-000000000001"); !errors.Is(err, ErrPlanMissing) {
		t.Fatal("invalid account accepted")
	}
	if _, err := store.SavePlanIntents(context.Background(), "synthetic", "00000000-0000-4000-8000-000000000001", "invalid"); !errors.Is(err, ErrPlanMissing) {
		t.Fatal("invalid plan id accepted")
	}
	if _, err := store.ListPlanIntents(context.Background(), "synthetic", "invalid", "00000000-0000-4000-8000-000000000001"); !errors.Is(err, ErrPlanMissing) {
		t.Fatal("invalid account accepted for list")
	}
}
