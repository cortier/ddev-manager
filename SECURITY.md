# Security policy

Please report suspected vulnerabilities privately through GitHub's **Report a vulnerability** feature instead of opening a public issue. Include the affected version, impact, and reproduction steps when possible.

Supported releases are the latest signed Firefox extension and companion release. Security fixes may require updating both components.

The companion executes only its fixed DDEV, Git, and IDE operations. It does not accept arbitrary commands over native messaging or HTTP. The cleanup HTTP listener is bound to `127.0.0.1`; registration uses a permission-restricted local socket and per-profile secrets.
