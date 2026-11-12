package picker

// pickerCSS is the layout of the selection page (task 82): two panes on a
// desktop screen and two screens on a phone. The layout lives here, not in
// inline styles, because a media query cannot override an inline style; the
// colours and the spacing that do not change with the screen stay inline in
// render.go.
//
// Every rule is scoped by data-picker, so two pages (the Library's and the
// Project's, both mounted) and the rest of the app are untouched.
const pickerCSS = `
[data-picker] {
	display: flex;
	flex-direction: row;
	flex: 1;
	min-width: 0;
	min-height: 0;
	padding: 8px 16px;
	box-sizing: border-box;
	font-size: 13px;
}
[data-picker] [data-picker-list] {
	display: flex;
	flex-direction: column;
	flex: 0 0 264px;
	min-width: 0;
	min-height: 0;
	padding-right: 12px;
	box-sizing: border-box;
	border-right: 1px solid var(--border);
}
[data-picker] [data-picker-list-header] {
	display: flex;
	align-items: center;
	gap: 8px;
	padding-bottom: 8px;
}
[data-picker] [data-picker-create] {
	width: 36px;
	height: 36px;
	padding: 0;
	box-sizing: border-box;
}
[data-picker] [data-picker-rows] {
	flex: 1;
	min-height: 0;
	overflow-y: auto;
	outline: none;
}
[data-picker] [data-picker-rows]:focus-visible {
	box-shadow: inset 0 0 0 1px var(--accent);
	border-radius: 4px;
}
[data-picker] [data-picker-details] {
	display: flex;
	flex-direction: column;
	flex: 1 1 auto;
	min-width: 0;
	min-height: 0;
	padding-left: 16px;
	overflow-y: auto;
	box-sizing: border-box;
}
[data-picker] [data-picker-row] {
	display: flex;
	align-items: center;
	gap: 8px;
	padding: 6px 4px;
	border-bottom: 1px solid var(--border);
	cursor: pointer;
	user-select: none;
}
[data-picker] [data-picker-row]:hover {
	background: var(--bg-hover);
}
[data-picker] [data-picker-row][data-picker-selected="1"] {
	background: var(--accent);
	color: var(--accent-text);
}
[data-picker] [data-picker-grip] {
	display: inline-flex;
	align-items: center;
	justify-content: center;
	width: 20px;
	height: 24px;
	flex-shrink: 0;
	cursor: grab;
	color: var(--text-muted);
}
[data-picker] [data-picker-grip]:active {
	cursor: grabbing;
}
[data-picker] [data-picker-name-row],
[data-picker] [data-picker-description-row] {
	display: flex;
	align-items: flex-start;
	gap: 8px;
}
[data-picker] [data-picker-description-row] {
	margin-top: 12px;
}
[data-picker] [data-picker-name-input],
[data-picker] [data-picker-description-input],
[data-picker] [data-picker-create-name] {
	width: 100%;
	box-sizing: border-box;
	padding: 4px 6px;
	border: 1px solid var(--border-input);
	border-radius: 4px;
	background: var(--input-bg);
	color: var(--text);
	font-family: inherit;
	font-size: 14px;
}
[data-picker] [data-picker-description-input] {
	resize: vertical;
}
[data-picker] [data-picker-description][data-empty="true"] {
	font-style: italic;
	color: var(--text-muted);
}
[data-picker] [data-picker-description] {
	color: var(--text-2);
}
[data-picker] [data-picker-details-actions] {
	display: flex;
	align-items: center;
	gap: 8px;
	margin-top: 18px;
}
[data-picker] [data-picker-back] {
	display: none;
	align-items: center;
	gap: 6px;
	padding: 6px 0;
	margin-bottom: 8px;
	cursor: pointer;
	color: var(--text-2);
	font-size: 12px;
	user-select: none;
}
[data-picker] [data-picker-row].is-dragging {
	opacity: 0.55;
	outline: 1px dashed var(--accent);
}
[data-picker] [data-picker-row].is-drop {
	box-shadow: inset 0 2px 0 var(--accent);
}
[data-picker][data-dragging="true"] [data-picker-row]:hover {
	box-shadow: inset 0 2px 0 var(--accent);
}
@media (max-width: 720px) {
	[data-picker] {
		padding: 4px 8px;
	}
	[data-picker] [data-picker-list] {
		flex: 1 1 auto;
		border-right: none;
		padding-right: 0;
	}
	[data-picker] [data-picker-details] {
		padding-left: 0;
	}
	[data-picker][data-screen="list"] [data-picker-details] {
		display: none;
	}
	[data-picker][data-screen="details"] [data-picker-list] {
		display: none;
	}
	[data-picker] [data-picker-back] {
		display: flex;
	}
	[data-picker] [data-picker-row] {
		min-height: 48px;
		padding: 8px 6px;
	}
	[data-picker] [data-picker-create] {
		width: 44px;
		height: 44px;
	}
	[data-picker] [data-picker-grip] {
		width: 32px;
		height: 44px;
		touch-action: none;
	}
	[data-picker] [data-picker-name] {
		font-size: 18px;
	}
}
`
