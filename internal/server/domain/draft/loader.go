package draft

import (
	"context"

	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/librarypin"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// Loader is what a Session reads inside the Draft's transaction: only the
// entities the operation touches, always within the Draft's own Project
// (ADR 0016). The repository implements it over the transaction; the domain
// knows no SQL.
type Loader interface {
	// Tag returns the Tag definition of the Draft's Project.
	Tag(ctx context.Context, tagID id.ID[tag.Tag]) (tag.Tag, error)
	// TagTypes returns the TagTypes of the given Tags of the Draft's
	// Project; a Tag not of it is left out.
	TagTypes(ctx context.Context, tagIDs []id.ID[tag.Tag]) (scene.TagTypes, error)
	// AllTagTypes returns the TagTypes of every Tag of the Draft's Project.
	AllTagTypes(ctx context.Context) (scene.TagTypes, error)
	// TagsOfDevice reports whether a Tag of the Draft's Project is supplied
	// by the Device.
	TagsOfDevice(ctx context.Context, deviceID id.ID[device.Device]) (bool, error)

	// AllTags returns every Tag definition of the Draft's Project.
	AllTags(ctx context.Context) ([]tag.Tag, error)
	// Scenes returns every Scene of the Draft's Project with its Widgets.
	Scenes(ctx context.Context) ([]scene.Scene, error)
	// Scene returns one Scene of the Draft's Project with its Widgets.
	Scene(ctx context.Context, sceneID id.ID[scene.Scene]) (scene.Scene, error)
	// PinnedWidgetType returns the WidgetType with the given ID from the
	// Library Releases the Draft's Project pins.
	PinnedWidgetType(ctx context.Context, typeID id.ID[library.WidgetType]) (library.ReleasedWidgetType, error)
	// Release returns the Library Release number of the Library.
	Release(ctx context.Context, libraryID id.ID[library.Library], number library.ReleaseNumber) (library.LibraryRelease, error)

	// AllDevices returns every Device of the Draft's Project.
	AllDevices(ctx context.Context) ([]device.Device, error)
	// Device returns one Device of the Draft's Project.
	Device(ctx context.Context, deviceID id.ID[device.Device]) (device.Device, error)
	// AllEnvironments returns every Environment of the Draft's Project.
	AllEnvironments(ctx context.Context) ([]device.Environment, error)
	// Environment returns one Environment of the Draft's Project.
	Environment(ctx context.Context, environmentID id.ID[device.Environment]) (device.Environment, error)
	// EnvironmentDevices returns every pair of the Draft's Project.
	EnvironmentDevices(ctx context.Context) ([]device.EnvironmentDevice, error)
	// Pair returns the parameters of the Device in the Environment, both of
	// the Draft's Project.
	Pair(ctx context.Context, environmentID id.ID[device.Environment], deviceID id.ID[device.Device]) (device.EnvironmentDevice, error)
	// PairsOfEnvironment returns the pairs the Environment holds.
	PairsOfEnvironment(ctx context.Context, environmentID id.ID[device.Environment]) ([]device.EnvironmentDevice, error)
	// PairsOfDevice returns the pairs the Device stands in.
	PairsOfDevice(ctx context.Context, deviceID id.ID[device.Device]) ([]device.EnvironmentDevice, error)

	// AllLibraryPins returns every Library Pin of the Draft's Project.
	AllLibraryPins(ctx context.Context) ([]librarypin.LibraryPin, error)
	// LibraryPin returns the pin of the Library in the Draft's Project.
	LibraryPin(ctx context.Context, libraryID id.ID[library.Library]) (librarypin.LibraryPin, error)

	// WidgetTypes returns every WidgetType of the Draft's Library, in
	// creation order.
	WidgetTypes(ctx context.Context) ([]library.WidgetType, error)
	// WidgetType returns one WidgetType of the Draft's Library.
	WidgetType(ctx context.Context, typeID id.ID[library.WidgetType]) (library.WidgetType, error)
}
