package uuidx

import (
	"fmt"

	"github.com/gofrs/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// StringToPGUUID converts a UUID string into a pgtype.UUID value.
func StringToPGUUID(s string) (pgtype.UUID, error) {
	var out pgtype.UUID
	if s == "" {
		return out, fmt.Errorf("empty uuid string")
	}

	id, err := uuid.FromString(s)
	if err != nil {
		return out, err
	}

	out.Bytes = id
	out.Valid = true
	return out, nil
}

// UUIDToPG converts a uuid.UUID into a pgtype.UUID.
func UUIDToPG(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{
		Bytes: id,
		Valid: true,
	}
}

// UUIDPtrToPG converts a *uuid.UUID into a pgtype.UUID (NULL when nil).
func UUIDPtrToPG(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{
		Bytes: *id,
		Valid: true,
	}
}

// PGToUUID converts a non-null pgtype.UUID into uuid.UUID.
func PGToUUID(p pgtype.UUID) (uuid.UUID, error) {
	if !p.Valid {
		return uuid.Nil, fmt.Errorf("uuid is null")
	}
	return p.Bytes, nil
}

// PGToUUIDPtr converts a pgtype.UUID into *uuid.UUID (nil when NULL).
func PGToUUIDPtr(p pgtype.UUID) (*uuid.UUID, error) {
	if !p.Valid {
		return nil, nil
	}
	v := p.Bytes
	// Need a uuid.UUID pointer, not *[16]byte.
	u := uuid.UUID(v)
	return &u, nil
}


