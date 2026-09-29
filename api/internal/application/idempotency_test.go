package application

import (
	"strings"
	"testing"
	"time"
)

func TestHashConfirmCommandCanonicalizesEquivalentInstants(t *testing.T) {
	first := ConfirmCommand{VehicleID: "AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA", DealershipID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", ServiceTypeID: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", StartAt: time.Date(2031, 3, 4, 9, 30, 0, 0, time.UTC), IdempotencyKey: "one"}
	second := first
	second.VehicleID = strings.ToLower(first.VehicleID)
	second.StartAt = time.Date(2031, 3, 4, 10, 30, 0, 0, time.FixedZone("plus-one", 3600))
	second.IdempotencyKey = "two"
	if HashConfirmCommand(first) != HashConfirmCommand(second) {
		t.Fatal("equivalent commands must have equal hashes")
	}
	second.ServiceTypeID = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	if HashConfirmCommand(first) == HashConfirmCommand(second) {
		t.Fatal("changed input must change hash")
	}
}
