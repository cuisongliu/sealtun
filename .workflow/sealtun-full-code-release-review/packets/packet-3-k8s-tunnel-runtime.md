Packet ID: packet-3-k8s-tunnel-runtime
Objective: Review Kubernetes and tunnel runtime behavior for scope isolation, cleanup safety, and resource visibility.
Files / sources: pkg/k8s, pkg/tunnel, relevant cmd call sites.
Do: Check labels/selectors, owner scope, secret redaction, NodePort/TCP/SSH/HTTPS resources, list performance, warnings.
Do not: Change cluster resource contracts without tests and docs.
Expected output: Result note with accepted/deferred findings and verification.
Verification: pkg/k8s and tunnel tests plus full gate.
