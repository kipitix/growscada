package draft

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/google/uuid"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
)

// commitContentFormatVersion is the version of the internal encoding of a
// Commit's content: like the outbox payload and a DraftChange, it is not a
// contract, and a change of the shape keeps reading the previous format
// until no Commit of it is left.
const commitContentFormatVersion = 1

// CommitContent is the immutable snapshot a Commit holds: the canonical
// ProjectFile of a Project's Draft, or the copies of a Library's Draft
// WidgetTypes — exactly one of them. It is stored encoded (CommitContent
// implements [json.Marshaler]) and is never changed: a Commit is a point of
// history.
type CommitContent struct {
	projectFile *projectv0.ProjectFile
	widgetTypes []library.ReleasedWidgetType
}

// NewProjectCommitContent returns the content of a Project's Commit over the
// canonical ProjectFile the Draft's builder assembled.
func NewProjectCommitContent(file projectv0.ProjectFile) CommitContent {
	return CommitContent{projectFile: &file}
}

// NewLibraryCommitContent returns the content of a Library's Commit over the
// copies of its Draft's WidgetTypes.
func NewLibraryCommitContent(widgetTypes []library.ReleasedWidgetType) CommitContent {
	return CommitContent{widgetTypes: slices.Clone(widgetTypes)}
}

// IsProject reports whether the content is a Project's.
func (c CommitContent) IsProject() bool { return c.projectFile != nil }

// IsLibrary reports whether the content is a Library's.
func (c CommitContent) IsLibrary() bool { return c.projectFile == nil && c.widgetTypes != nil }

// ProjectFile returns the content's ProjectFile; false for a Library's
// content.
func (c CommitContent) ProjectFile() (projectv0.ProjectFile, bool) {
	if c.projectFile == nil {
		return projectv0.ProjectFile{}, false
	}
	return *c.projectFile, true
}

// WidgetTypes returns the content's WidgetTypes; false for a Project's
// content.
func (c CommitContent) WidgetTypes() ([]library.ReleasedWidgetType, bool) {
	if c.projectFile != nil {
		return nil, false
	}
	return slices.Clone(c.widgetTypes), true
}

// commitContentDocument is the stored form of a CommitContent.
type commitContentDocument struct {
	Format      int                          `json:"format"`
	Kind        string                       `json:"kind"`
	ProjectFile *projectv0.ProjectFile       `json:"project_file,omitempty"`
	WidgetTypes []releasedWidgetTypeDocument `json:"widget_types,omitempty"`
}

// releasedWidgetTypeDocument is the stored form of a ReleasedWidgetType; the
// field names repeat the release tables.
type releasedWidgetTypeDocument struct {
	ID             uuid.UUID           `json:"id"`
	LibraryID      uuid.UUID           `json:"library_id"`
	Name           string              `json:"name"`
	HtmlTemplate   string              `json:"html_template"`
	Script         string              `json:"script"`
	ScriptLanguage string              `json:"script_language"`
	DefaultWidth   int                 `json:"default_width"`
	DefaultHeight  int                 `json:"default_height"`
	InputPorts     []inputPortDocument `json:"input_ports"`
}

// inputPortDocument is the stored form of one InputPort.
type inputPortDocument struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	TypeHint    string `json:"type_hint"`
}

// MarshalJSON encodes the content with the format version and the kind, so a
// reader needs no scope to decode it.
func (c CommitContent) MarshalJSON() ([]byte, error) {
	doc := commitContentDocument{Format: commitContentFormatVersion}
	switch {
	case c.projectFile != nil:
		doc.Kind = "project"
		doc.ProjectFile = c.projectFile
	case c.widgetTypes != nil:
		doc.Kind = "library"
		doc.WidgetTypes = make([]releasedWidgetTypeDocument, len(c.widgetTypes))
		for i, wt := range c.widgetTypes {
			ports := wt.InputPorts()
			rows := make([]inputPortDocument, len(ports))
			for j, p := range ports {
				rows[j] = inputPortDocument{
					Name: p.Name().String(), Description: p.Description(), TypeHint: p.TypeHint().String(),
				}
			}
			doc.WidgetTypes[i] = releasedWidgetTypeDocument{
				ID:             wt.ID().UUID(),
				LibraryID:      wt.LibraryID().UUID(),
				Name:           wt.Name().String(),
				HtmlTemplate:   wt.HtmlTemplate().String(),
				Script:         wt.Script().String(),
				ScriptLanguage: wt.ScriptLanguage().String(),
				DefaultWidth:   wt.DefaultSize().Width(),
				DefaultHeight:  wt.DefaultSize().Height(),
				InputPorts:     rows,
			}
		}
	default:
		return nil, fmt.Errorf("cannot encode commit content: it holds neither a project file nor widget types")
	}
	encoded, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("cannot encode commit content: %w", err)
	}
	return encoded, nil
}

// UnmarshalJSON decodes the content as MarshalJSON wrote it. A format this
// build does not know is an error: a Commit is history, never guessed.
func (c *CommitContent) UnmarshalJSON(data []byte) error {
	var doc commitContentDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("cannot decode commit content: %w", err)
	}
	if doc.Format != commitContentFormatVersion {
		return fmt.Errorf("commit content format %d is not the supported %d", doc.Format, commitContentFormatVersion)
	}
	switch doc.Kind {
	case "project":
		if doc.ProjectFile == nil {
			return fmt.Errorf("cannot decode commit content: kind %q without a project file", doc.Kind)
		}
		*c = NewProjectCommitContent(*doc.ProjectFile)
		return nil
	case "library":
		types := make([]library.ReleasedWidgetType, len(doc.WidgetTypes))
		for i, row := range doc.WidgetTypes {
			ports := make([]library.InputPort, len(row.InputPorts))
			for j, p := range row.InputPorts {
				name, err := library.NewInputPortNameFromStorage(p.Name)
				if err != nil {
					return fmt.Errorf("cannot decode commit content: input port %q: %w", p.Name, err)
				}
				hint, err := library.NewPortTypeHint(p.TypeHint)
				if err != nil {
					return fmt.Errorf("cannot decode commit content: input port %q: %w", p.Name, err)
				}
				ports[j] = library.NewInputPort(name, p.Description, hint)
			}
			name, err := library.NewWidgetTypeName(row.Name)
			if err != nil {
				return fmt.Errorf("cannot decode commit content: widget type %s: %w", row.ID, err)
			}
			html, err := library.NewHtmlTemplate(row.HtmlTemplate)
			if err != nil {
				return fmt.Errorf("cannot decode commit content: widget type %s: %w", row.ID, err)
			}
			script, err := library.NewScript(row.Script)
			if err != nil {
				return fmt.Errorf("cannot decode commit content: widget type %s: %w", row.ID, err)
			}
			language, err := library.NewScriptLanguage(row.ScriptLanguage)
			if err != nil {
				return fmt.Errorf("cannot decode commit content: widget type %s: %w", row.ID, err)
			}
			size, err := library.NewSize(row.DefaultWidth, row.DefaultHeight)
			if err != nil {
				return fmt.Errorf("cannot decode commit content: widget type %s: %w", row.ID, err)
			}
			copied, err := library.NewReleasedWidgetType(
				id.NewID(id.IDWithUUID[library.WidgetType](row.ID)),
				id.NewID(id.IDWithUUID[library.Library](row.LibraryID)),
				name, html, script, language, size, ports,
			)
			if err != nil {
				return fmt.Errorf("cannot decode commit content: widget type %s: %w", row.ID, err)
			}
			types[i] = copied
		}
		*c = NewLibraryCommitContent(types)
		return nil
	default:
		return fmt.Errorf("cannot decode commit content: unknown kind %q", doc.Kind)
	}
}
