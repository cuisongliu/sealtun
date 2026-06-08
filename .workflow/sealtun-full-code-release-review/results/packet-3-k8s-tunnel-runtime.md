Packet: packet-3-k8s-tunnel-runtime
Status: completed

Reviewed:
- `pkg/k8s/client.go`
- `pkg/k8s/client_test.go`
- tunnel provisioning call sites in `cmd/apply.go`, `cmd/expose.go`, dashboard mutations

Accepted findings:
- No P0/P1 Kubernetes ownership or secret leakage issue found.
- Managed label checks protect update/delete paths for Deployment, Service, Ingress, Certificate, Issuer, and Secret.
- TCP/SSH NodePort service preserves existing NodePort on update.
- Resources payload reports Secret metadata only and tests assert secret data does not leak.

Deferred findings:
- `pkg/k8s/client.go` is large and would benefit from future module splitting, but a broad refactor would be high churn and not necessary for this release.

Verification:
- Covered by `go test ./pkg/k8s` and full release gate.
