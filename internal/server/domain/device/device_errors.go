package device

import "errors"

var (
	// ErrDeviceNotFound is returned when the Project holds no such Device.
	ErrDeviceNotFound = errors.New("device not found")
	// ErrDeviceConflict is returned by the Device's change methods when the
	// expected version is not the current one (an edit conflict), and by Save
	// when the stored version changed after the Device was read (a write
	// race).
	ErrDeviceConflict = errors.New("device version conflict")
	// ErrDeviceNameTaken is returned by Save when another Device of the
	// Project already has the name: within its Project a Device's name is its
	// unique natural key (ADR 0003).
	ErrDeviceNameTaken = errors.New("device name already taken")
	// ErrDeviceInUse is returned by Delete while a Tag of the Project is
	// supplied by the Device, or while the Device stands in an Environment:
	// references are refused, never cascaded (ADR 0008).
	ErrDeviceInUse = errors.New("device in use")

	// ErrEnvironmentNotFound is returned when the Project holds no such
	// Environment.
	ErrEnvironmentNotFound = errors.New("environment not found")
	// ErrEnvironmentConflict is returned by the Environment's change methods
	// when the expected version is not the current one, and by Save when the
	// stored version changed after the Environment was read.
	ErrEnvironmentConflict = errors.New("environment version conflict")
	// ErrEnvironmentNameTaken is returned by Save when another Environment of
	// the Project already has the name.
	ErrEnvironmentNameTaken = errors.New("environment name already taken")
	// ErrEnvironmentInUse is returned by Delete while the Environment still
	// holds the pairs of its Devices: references are refused, never cascaded
	// (ADR 0008).
	ErrEnvironmentInUse = errors.New("environment in use")

	// ErrEnvironmentDeviceNotFound is returned when the Environment holds no
	// parameters for the Device.
	ErrEnvironmentDeviceNotFound = errors.New("environment device not found")
	// ErrEnvironmentDeviceConflict is returned by the EnvironmentDevice's
	// change methods when the expected version is not the current one, and by
	// Save and Delete when the stored version changed after the pair was
	// read.
	ErrEnvironmentDeviceConflict = errors.New("environment device version conflict")
	// ErrEnvironmentDeviceExists is returned by Save of a new pair when the
	// Environment already holds the Device: the pair identifies one
	// EnvironmentDevice.
	ErrEnvironmentDeviceExists = errors.New("environment device already exists")
)
