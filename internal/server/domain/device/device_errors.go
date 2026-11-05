package device

import "errors"

var (
	// ErrDeviceNotFound is returned when the Project holds no such Device.
	ErrDeviceNotFound = errors.New("device not found")
	// ErrDeviceNameTaken is returned when another Device of the
	// Project already has the name: within its Project a Device's name is its
	// unique natural key (ADR 0003).
	ErrDeviceNameTaken = errors.New("device name already taken")
	// ErrDeviceInUse is returned by a delete of the Draft while a Tag of the Project is
	// supplied by the Device, or while the Device stands in an Environment:
	// references are refused, never cascaded (ADR 0008).
	ErrDeviceInUse = errors.New("device in use")

	// ErrEnvironmentNotFound is returned when the Project holds no such
	// Environment.
	ErrEnvironmentNotFound = errors.New("environment not found")
	// ErrEnvironmentNameTaken is returned when another Environment of
	// the Project already has the name.
	ErrEnvironmentNameTaken = errors.New("environment name already taken")
	// ErrEnvironmentInUse is returned by a delete of the Draft while the Environment still
	// holds the pairs of its Devices: references are refused, never cascaded
	// (ADR 0008).
	ErrEnvironmentInUse = errors.New("environment in use")

	// ErrEnvironmentDeviceNotFound is returned when the Environment holds no
	// parameters for the Device.
	ErrEnvironmentDeviceNotFound = errors.New("environment device not found")
	// ErrEnvironmentDeviceExists is returned by a write of a new pair when the
	// Environment already holds the Device: the pair identifies one
	// EnvironmentDevice.
	ErrEnvironmentDeviceExists = errors.New("environment device already exists")
)
