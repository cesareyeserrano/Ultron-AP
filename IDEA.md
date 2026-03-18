# Project Idea

Ultron-AP is a professional monitoring and management dashboard for the Raspberry Pi. It solves the problem of operators needing reliable, low-friction, real-time visibility into system health without requiring SSH access or heavyweight tooling. The primary user is a Raspberry Pi operator/admin who needs to understand critical system state and act quickly — restarting containers, viewing logs, configuring alerts — from a single web interface.

The project provides real-time monitoring of CPU, RAM, Disk, Network, and temperature via SSE; Docker and Systemd service controls; a threshold-based alert engine with Telegram and Email notifications; hardware integration with Pironman 5; and security features including CSRF protection, bcrypt sessions, brute-force protection, and a full action audit trail. A privilege-separation model keeps the web process unprivileged while delegating host-level actions to a root-owned helper over a Unix socket.

Built with a zero-runtime-dependency philosophy using Go, HTMX, Tailwind CSS, and SQLite. Targets ARM64 (Raspberry Pi) with a ~15MB RAM footprint.
