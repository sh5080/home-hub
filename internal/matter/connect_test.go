package matter

import (
	"context"
	"testing"
)

// TestDialGoMatterMissingFabric checks the load-fabric failure path: a bad
// fabric-store path surfaces as an error instead of a nil-deref.
func TestDialGoMatterMissingFabric(t *testing.T) {
	_, err := DialGoMatter(context.Background(), GoMatterConfig{
		FabricStore: "/nonexistent/fabric.json",
		NodeID:      1,
		Address:     "127.0.0.1:5540",
		Endpoint:    1,
	})
	if err == nil {
		t.Fatal("expected an error when the fabric store is missing")
	}
}
