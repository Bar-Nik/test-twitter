package app

import (
	"time"

	"github.com/gofrs/uuid/v5"
)

type Tweet struct {
	Id        uuid.UUID
	Text      string
	CreatedAt time.Time
	UpdatedAt time.Time
	UserId    uuid.UUID
}
