# GrowSCADA

A web-based SCADA (Supervisory Control and Data Acquisition) system: it exposes process variables (Tags) and lets operators view and interact with them through custom-built visual screens (Scenes) made of reusable widgets.

## Language

### Data

**Tag**:
The atomic process variable — an identifier, name, type, current value, and quality. Aggregate.
_Avoid_: variable, point, signal

**Quality**:
A Tag's reliability indicator: `Bad`, `Uncertain`, or `Good`. Describes how trustworthy the value is, never where it came from. There is no "unknown" Quality — every Tag always has one of the three, stated explicitly by whoever creates or updates it.
_Avoid_: status, state; simulated/forced/unknown as a Quality

**TagType**:
A Tag's data type: `String`, `Boolean`, or `Integer`. Governs which values the Tag accepts. There is no "unknown" TagType.
_Avoid_: analog/discrete, data type

**Device**:
A source of Tag values — real equipment, a protocol adapter, or a simulator. Whether it is a simulator is descriptive metadata of the Device, not a Quality. Not yet modelled.
_Avoid_: source, adapter, PLC

### Visualization

**WidgetType**:
A reusable template defining how a widget renders: an HTML template plus a script, a default size, and the InputPorts it exposes. Aggregate.
_Avoid_: component, widget definition

**InputPort**:
A named, typed input slot declared on a WidgetType, referenced in its template/script as `input.<name>`. Its type hint either names one TagType or accepts any TagType — "any" is a property of the hint, not a TagType.
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
