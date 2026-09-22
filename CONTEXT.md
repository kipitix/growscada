# GrowSCADA

A web-based SCADA (Supervisory Control and Data Acquisition) system: it exposes process variables (Tags) and lets operators view and interact with them through custom-built visual screens (Scenes) made of reusable widgets.

## Language

### Data

**Tag**:
The atomic process variable — an identifier, name, type, current value, and quality. Aggregate.
_Avoid_: variable, point, signal

**Quality**:
A Tag's reliability indicator: `Unknown`, `Bad`, `Uncertain`, `Good`, or `Simulated` (manually overridden value).
_Avoid_: status, state

**TagType**:
A Tag's data type: `Unknown`, `String`, `Boolean`, or `Integer`. Governs which values the Tag accepts.
_Avoid_: analog/discrete, data type

### Visualization

**WidgetType**:
A reusable template defining how a widget renders: an HTML template plus a script, a default size, and the InputPorts it exposes. Aggregate.
_Avoid_: component, widget definition

**InputPort**:
A named, typed input slot declared on a WidgetType, referenced in its template/script as `input.<name>`. Its type hint may accept any Tag type.
_Avoid_: parameter, slot

**Widget**:
A placed instance of a WidgetType on a Scene — positioned, sized, rotated, optionally labelled, and bound to Tags via PortBindings. Entity of the Scene aggregate: it has no identity or version outside the Scene that contains it.
_Avoid_: element, control

**PortBinding**:
The link between one of a Widget's InputPorts and the Tag that feeds it.
_Avoid_: mapping, connection

**Scene**:
A named canvas (a page inside a project) with fixed dimensions, a static HTML background, and the Widgets placed on it. Aggregate — the sole consistency boundary for itself and its Widgets.
_Avoid_: screen, mnemonic, page, display

### Cross-cutting

**Version**:
The optimistic-concurrency counter carried by every aggregate (Tag, WidgetType, Scene), incremented on each committed change.
_Avoid_: revision, etag

**Client**:
Identity marker for a connected SSE (Server-Sent Events) client, used by connection-lifecycle domain events. Not yet a full aggregate — no access rights or behavior defined.
_Avoid_: session, connection, user
