// Copyright (c) 2026 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package generic

import (
	"fmt"
	"testing"
)

func TestObjectMetadata_Key(t *testing.T) {
	m := ObjectMetadata{
		Group:     "apps",
		Version:   "v1",
		Kind:      "Deployment",
		Namespace: "ns",
		Name:      "name",
	}
	want := "apps/v1/Deployment/ns/name"
	if got := m.Key(); got != want {
		t.Errorf("Key() = %q, want %q", got, want)
	}
}

func TestAddUpdate_DedupWithKeyFunc(t *testing.T) {
	type item struct {
		Name string `json:"name"`
		Val  string `json:"val"`
	}
	b := NewGenericBundle(WithKeyFunc(func(i item) string { return i.Name }))

	added, err := b.AddUpdate(item{Name: "a", Val: "1"})
	if err != nil || !added {
		t.Fatalf("first AddUpdate: added=%v err=%v", added, err)
	}
	added, err = b.AddUpdate(item{Name: "a", Val: "2"})
	if err != nil || !added {
		t.Fatalf("dedup AddUpdate: added=%v err=%v", added, err)
	}
	if len(b.Update) != 1 {
		t.Fatalf("expected 1 update after dedup, got %d", len(b.Update))
	}
	if b.Update[0].Val != "2" {
		t.Errorf("expected latest value 2, got %s", b.Update[0].Val)
	}
}

func TestAddDelete_DedupByKey(t *testing.T) {
	b := NewGenericBundle[string]()
	meta := ObjectMetadata{Group: "g", Version: "v1", Kind: "K", Namespace: "ns", Name: "n"}

	added, err := b.AddDelete(meta)
	if err != nil || !added {
		t.Fatalf("first AddDelete: added=%v err=%v", added, err)
	}
	meta2 := meta
	meta2.ID = "updated-id"
	added, err = b.AddDelete(meta2)
	if err != nil || !added {
		t.Fatalf("dedup AddDelete: added=%v err=%v", added, err)
	}
	if len(b.Delete) != 1 {
		t.Fatalf("expected 1 delete after dedup, got %d", len(b.Delete))
	}
	if b.Delete[0].ID != "updated-id" {
		t.Errorf("expected ID updated-id, got %s", b.Delete[0].ID)
	}
}

func TestAddUpdate_ReplaceExceedsSizeLimit(t *testing.T) {
	type item struct {
		Name string `json:"name"`
		Data string `json:"data"`
	}
	b := NewGenericBundle(WithKeyFunc(func(i item) string { return i.Name }))

	small := item{Name: "target", Data: "small"}
	added, err := b.AddUpdate(small)
	if err != nil || !added {
		t.Fatalf("add small target: added=%v err=%v", added, err)
	}

	// Fill the bundle until it cannot accept another filler.
	for i := 0; ; i++ {
		filler := item{Name: fmt.Sprintf("f%d", i), Data: string(make([]byte, 50*1024))}
		added, err = b.AddUpdate(filler)
		if err != nil {
			t.Fatalf("add filler %d: %v", i, err)
		}
		if !added {
			break
		}
		if i > 30 {
			t.Fatal("bundle never filled")
		}
	}

	before, err := b.Size()
	if err != nil {
		t.Fatalf("Size() before replace: %v", err)
	}

	large := item{Name: "target", Data: string(make([]byte, 200*1024))}
	added, err = b.AddUpdate(large)
	if err != nil {
		t.Fatalf("replace with large: %v", err)
	}
	if added {
		t.Fatal("expected keyed replace to return added=false when size exceeds MaxBundleBytes")
	}

	after, err := b.Size()
	if err != nil {
		t.Fatalf("Size() after replace: %v", err)
	}
	if after != before {
		t.Errorf("bundle size changed after rejected replace: before=%d after=%d", before, after)
	}

	found := false
	for _, u := range b.Update {
		if u.Name == "target" {
			found = true
			if u.Data != "small" {
				t.Errorf("expected original small target to remain, got data len=%d", len(u.Data))
			}
		}
	}
	if !found {
		t.Fatal("target entry missing after rejected replace")
	}
}

func TestAddDelete_ReplaceExceedsSizeLimit(t *testing.T) {
	b := NewGenericBundle[string]()

	target := ObjectMetadata{
		Group: "g", Version: "v1", Kind: "K", Namespace: "ns", Name: "target", ID: "small",
	}
	added, err := b.AddDelete(target)
	if err != nil || !added {
		t.Fatalf("add small target: added=%v err=%v", added, err)
	}

	for i := 0; ; i++ {
		filler := ObjectMetadata{
			Group: "g", Version: "v1", Kind: "K", Namespace: "ns",
			Name: fmt.Sprintf("f%d", i), ID: string(make([]byte, 2*1024)),
		}
		added, err = b.AddDelete(filler)
		if err != nil {
			t.Fatalf("add filler %d: %v", i, err)
		}
		if !added {
			break
		}
		if i > 500 {
			t.Fatal("bundle never filled")
		}
	}

	before, err := b.Size()
	if err != nil {
		t.Fatalf("Size() before replace: %v", err)
	}

	large := target
	large.ID = string(make([]byte, 100*1024))
	added, err = b.AddDelete(large)
	if err != nil {
		t.Fatalf("replace with large: %v", err)
	}
	if added {
		t.Fatal("expected keyed delete replace to return added=false when size exceeds MaxBundleBytes")
	}

	after, err := b.Size()
	if err != nil {
		t.Fatalf("Size() after replace: %v", err)
	}
	if after != before {
		t.Errorf("bundle size changed after rejected replace: before=%d after=%d", before, after)
	}
	for _, d := range b.Delete {
		if d.Name == "target" && d.ID != "small" {
			t.Errorf("expected original small ID to remain, got len=%d", len(d.ID))
		}
	}
}
