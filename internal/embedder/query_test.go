package embedder

import (
	"context"
	"reflect"
	"testing"
)

func TestQueryEmbedderAppliesInstruction(t *testing.T) {
	fake := &fakeEmbedder{}
	q := NewQueryEmbedder(fake, "retrieve relevant passages")

	vec, err := q.EmbedQuery(context.Background(), "where is the hospital bag")
	if err != nil {
		t.Fatalf("EmbedQuery: %v", err)
	}
	want := QueryText("retrieve relevant passages", "where is the hospital bag")
	if got := fake.inputs(); len(got) != 1 || got[0] != want {
		t.Errorf("embedded %v, want exactly [%q]", got, want)
	}
	if !reflect.DeepEqual(vec, fakeVec(want)) {
		t.Errorf("vec = %v, want the vector for the wrapped query", vec)
	}
}

func TestQueryEmbedderPropagatesFailure(t *testing.T) {
	q := NewQueryEmbedder(&fakeEmbedder{fail: true}, "")
	if _, err := q.EmbedQuery(context.Background(), "anything"); err == nil {
		t.Error("EmbedQuery with failing client returned nil error")
	}
}
