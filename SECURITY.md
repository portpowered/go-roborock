# Security

Do not put account tokens, passwords, email codes, device local keys, MQTT secrets, camera passwords, or unredacted captures into issues or pull requests. Use GitHub's private vulnerability reporting for this repository when available: https://github.com/portpowered/go-roborock/security/advisories/new.

The default MQTT transport verifies TLS. Custom HTTP clients and MQTT dial hooks are caller-owned trust boundaries. Device changes and uncertain movement commands are never automatically retried.

The first release series receives fixes on main. Report the module version, Go version, affected operation, and a sanitized reproduction without credentials.
