package connectors

import "errors"

var (
	ErrInvalidInventoryPath  = errors.New("invalid inventory file path")
	ErrInventoryFileNotFound = errors.New("inventory file not found")
)
