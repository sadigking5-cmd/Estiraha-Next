# Phase 0 Decision Gate Notes (`docs/decisions/PHASE0_GATES.md`)

Before Phase 1 implementation locks UI/multimedia binaries, three mandatory engineering gates must execute on Windows 10/11:
1. **Wails v3 Gate (`OPEN-001`):** Validate RTL layout rendering, WebView2 custom scheme image serving from Smart Image Engine, and System Tray lifecycle stability.
2. **mpv / libmpv Gate (`OPEN-002`):** Validate LGPLv2.1 dynamic linking compliance and Arabic subtitle (`ASS`/`SRT`) rendering fidelity; maintain VLC external fallback.
3. **ffprobe Gate (`OPEN-003`):** Validate LGPL redistribution compliance and batch JSON stream-probe latency across synthetic multi-thousand file queues.
