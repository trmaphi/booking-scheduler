package application

import (
	"crypto/sha256"
	"encoding/binary"
	"strings"
)

// HashConfirmCommand returns a stable digest of caller-controlled booking
// choices. The idempotency key is deliberately excluded from the payload.
func HashConfirmCommand(command ConfirmCommand) [32]byte {
	hash := sha256.New()
	fields := []string{
		strings.ToLower(strings.TrimSpace(command.VehicleID)),
		strings.ToLower(strings.TrimSpace(command.DealershipID)),
		strings.ToLower(strings.TrimSpace(command.ServiceTypeID)),
		command.StartAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
	}
	var length [8]byte
	for _, field := range fields {
		binary.BigEndian.PutUint64(length[:], uint64(len(field)))
		_, _ = hash.Write(length[:])
		_, _ = hash.Write([]byte(field))
	}
	var result [32]byte
	copy(result[:], hash.Sum(nil))
	return result
}
