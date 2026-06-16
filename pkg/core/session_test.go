package core

import (
	"context"
	"testing"
)

func TestWithClientAndClientFrom(t *testing.T) {
	ctx := WithClient(context.Background(), nil)
	if ClientFrom(ctx) != nil {
		t.Fatal("expected nil client from nil set")
	}

	ctx = context.Background()
	if ClientFrom(ctx) != nil {
		t.Fatal("expected nil client from empty context")
	}
}

func TestWithSessionAndSessionFrom(t *testing.T) {
	ctx := WithSession(context.Background(), (*AgentSession)(nil))
	if SessionFrom(ctx) != nil {
		t.Fatal("expected nil from nil session")
	}

	ctx = context.Background()
	if SessionFrom(ctx) != nil {
		t.Fatal("expected nil from empty context")
	}
}
