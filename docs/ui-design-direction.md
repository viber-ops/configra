# Management UI direction

The console serves operators who locate a configuration or credential, inspect its current state, and make a controlled change. The visual reference is the calm navigation, readable item lists, and focused detail pane of 1Password; Configra retains its own identity and Environment-based model.

## Tokens

- Paper: `#ffffff`; canvas: `#f3f5f9`; ink: `#162238`.
- Navigation: `#142a4a`; action blue: `#126ef2`; secondary text: `#75839c`.
- Display: SF Pro Display / Segoe UI Variable Display; body: SF Pro Text / Segoe UI / PingFang SC; identifiers: SF Mono / Consolas.
- Display text uses restrained 24–28 px headings; body and controls use 13–14 px; identifiers use 11–12 px. Use 8 px spacing steps, 10–12 px panel corners, and clear focus rings.

## Layout

```text
┌──────────────────┬───────────────────────────────────────────────┐
│ Configra         │ Search resources                 Preferences  │
│                  ├────────────────┬──────────────────────────────┤
│ Workspace        │ Item list      │ Selected resource            │
│  Environments    │ Name + key     │ Environment / revision       │
│  Configs         │ Filter + state │                              │
│  Vault           │                │ Fields or configuration      │
│                  │                │                              │
│ Security & logs  │                │ Contextual actions           │
│                  │                │                              │
│ Account          │                │                              │
└──────────────────┴────────────────┴──────────────────────────────┘
```

The signature is a compact resource identity tile: a quiet icon, readable name,
and a secondary immutable key, reused in lists and detail headers. Keep secrets
concealed until explicitly requested, limit long field/Environment inventories,
and group actions by the resource being changed. Narrow screens use one focused
pane with an explicit way back to the list.

The plan deliberately removes oversized technical eyebrows and repeated badges
that compete with names. Dark mode uses the same hierarchy, rather than making
every surface equally dark. Keyboard navigation, reduced motion, and empty/error
states are part of the design.
