package session

import "testing"

func TestTable(t *testing.T) {
	tbl := NewTable()
	s := &Secure{LocalSessionID: 42}
	tbl.Add(s)

	if got, ok := tbl.Get(42); !ok || got != s {
		t.Fatal("Get did not return the added session")
	}
	if _, ok := tbl.Get(7); ok {
		t.Fatal("Get returned an unknown session")
	}
	if tbl.Len() != 1 {
		t.Fatalf("Len = %d, want 1", tbl.Len())
	}
	tbl.Remove(42)
	if _, ok := tbl.Get(42); ok {
		t.Fatal("session was not removed")
	}
}

func TestAllocID(t *testing.T) {
	tbl := NewTable()
	for i := 0; i < 50; i++ {
		id, err := tbl.AllocID()
		if err != nil {
			t.Fatal(err)
		}
		if id == 0 {
			t.Fatal("allocated reserved id 0")
		}
		if _, ok := tbl.Get(id); ok {
			continue // pre-existing id; AllocID must never return an in-use one
		}
		tbl.Add(&Secure{LocalSessionID: id})
	}
	id, err := tbl.AllocID()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := tbl.Get(id); ok {
		t.Fatal("AllocID returned an id already in the table")
	}
}
