package draft

import (
	"context"

	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/libraryimport"
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
	// ImportedWidgetType returns the WidgetType with the given ID from the
	// saved states the Draft's Project imports.
	ImportedWidgetType(ctx context.Context, typeID id.ID[library.WidgetType]) (library.ReleasedWidgetType, error)
	// LibraryCommit returns the saved state number of the Library, with the
	// WidgetTypes it holds: what an import names (task 85_6). A number the
	// Library's Draft does not have is draft.ErrCommitNotFound.
	LibraryCommit(ctx context.Context, libraryID id.ID[library.Library], number int) (libraryimport.Commit, error)
	// LockLibrary locks the live Library's row FOR SHARE in the Draft's
	// transaction: an import of it cannot then be written after its deletion
	// begins (ADR 0018). A Library that is gone or being deleted is
	// library.ErrLibraryNotFound.
	LockLibrary(ctx context.Context, libraryID id.ID[library.Library]) error

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

	// AllLibraryImports returns every Library Import of the Draft's Project.
	AllLibraryImports(ctx context.Context) ([]libraryimport.LibraryImport, error)
	// LibraryImport returns the import of the Library in the Draft's Project.
	LibraryImport(ctx context.Context, libraryID id.ID[library.Library]) (libraryimport.LibraryImport, error)

	// WidgetTypes returns every WidgetType of the Draft's Library, in
	// creation order.
	WidgetTypes(ctx context.Context) ([]library.WidgetType, error)
	// WidgetType returns one WidgetType of the Draft's Library.
	WidgetType(ctx context.Context, typeID id.ID[library.WidgetType]) (library.WidgetType, error)
}
