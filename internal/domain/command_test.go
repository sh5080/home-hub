package domain

import "testing"

func TestSetOn(t *testing.T) {
	c := SetOn("d", true)
	if c.DeviceID != "d" || c.Action != ActionSetOn {
		t.Fatalf("SetOn = %+v", c)
	}
	on, ok := c.Value.(bool)
	if !ok || !on {
		t.Fatalf("value = %v (%T)", c.Value, c.Value)
	}
}

func TestSetPosition(t *testing.T) {
	c := SetPosition("d", 37)
	if c.Action != ActionSetPosition {
		t.Fatalf("action = %v", c.Action)
	}
	p, ok := c.Value.(int)
	if !ok || p != 37 {
		t.Fatalf("value = %v (%T)", c.Value, c.Value)
	}
}
