# Custom Reflector Operator

[![Go Version](https://img.shields.io/github/gomod/go-version/slices-ri/custom-reflector-operator)](https://golang.org)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-v1.28%2B-blue)](https://kubernetes.io)

An automated, native Kubernetes Go Operator built with `sigs.k8s.io/controller-runtime` that enables **Generic Resource Reflection** and **Closed-Loop Control** for Custom Resource Definitions (CRDs) across multi-cluster environments (e.g., **Liqo** federated clusters).

---

## Overview & Problem Statement

Native multi-cluster reflection tools like **Liqo (v0.10.0+)** are designed for *Pod Offloading* and natively reflect core Kubernetes primitives (`Pod`, `Service`, `ConfigMap`, `Secret`). However, they do **not** reflect non-core **Custom Resource Definitions (CRDs)** (such as S4T IoT devices or Crossplane Custom Resources).

When a Custom Resource (e.g. `S4TDevice`) is applied on a primary cluster (*Home/Master*), native Liqo ignores it, leaving it isolated locally without forwarding it to the remote cluster (*Foreign/Edge*) where the physical actuator resides.

The **Custom Reflector Operator** solves this architectural gap by providing a **Dual-Client Reconciler** that:
1. **Spec Reflection (Forward Sync)**: Intercepts custom resource creations/updates on the Home Cluster and automatically clones the `spec` payload into the remote shadow namespace on the Foreign Cluster via the Liqo VPN overlay (`10.202.0.1`).
2. **Status Reflection (Backward Sync / Closed-Loop Control)**: Listens for status changes on the Foreign Cluster (e.g. `status.phase: Ready`) and retro-propagates the status update back to the original resource on the Home Cluster.

---

## Closed-Loop Architecture

```
+-----------------------------------------------------------------------------------+
|                               HOME CLUSTER (MASTER)                               |
|                                                                                   |
|  +------------------+         +-------------------------------+                   |
|  |  User / App      |         |   Custom Reflector Operator   |                   |
|  |  kubectl apply   | ------> |     (Dual-Client Reconciler)  |                   |
|  |  (S4TDevice Spec)|         |                               |                   |
|  +------------------+         |  +-------------------------+  |                   |
|           |                   |  | HomeClient (K8s Master) |  |                   |
|           v                   |  +-------------------------+  |                   |
|    API Server Home            |  | ForeignClient (Edge1)   |  |                   |
|    (test-offloading)          |  +-------------------------+  |                   |
|           ^                   +-------------------------------+                   |
|           |                               |         ^                             |
|           | (3) Status Reflection         |         |                             |
|           |     (Patch Phase: Ready)      | (1) Spec Reflection                   |
|           |                               |     (Clone Spec)                      |
|           |                               v         |                             |
+-----------|-------------------------------|---------|-----------------------------+
            |                               |         |
            |     = = = = = = = = = = = = = | = = = = | = = = = = = = = = = = = = =
            |     Network Fabric Liqo (VPN WireGuard Overlay Endpoint: 10.202.0.1)
            |     = = = = = = = = = = = = = | = = = = | = = = = = = = = = = = = = =
            |                               v         |
+-----------|-----------------------------------------|-----------------------------+
|           |                     FOREIGN CLUSTER (EDGE1)                           |
|           |                                                                       |
|           |                   API Server Remote (Edge1)                           |
|           |              (test-offloading-master-b616c8)                          |
|           |                               |                                       |
|           |                               v                                       |
|           |                   Physical Actuator / Crossplane                      |
|           |                   (Configures Edge Hardware)                          |
|           |                               |                                       |
|           +-------------------------------+ (2) Set Status Phase: Ready           |
|                                                                                   |
+-----------------------------------------------------------------------------------+
```

---

## Quick Start & Deployment

### Prerequisites
- Kubernetes cluster (v1.26+)
- Go (v1.22+)
- Docker / Containerd

### 1. Build Container Image
```bash
docker build -t custom-reflector-operator:latest .
```

### 2. Create Remote Cluster Secret & Deploy Operator
```bash
# Create Kubernetes Secret containing remote cluster Kubeconfig
kubectl create secret generic foreign-kubeconfig-secret \
  --from-file=edge1.yaml=/path/to/remote/edge1.yaml \
  -n liqo-system

# Deploy the operator
kubectl apply -f deploy/deployment.yaml
```

---


## 📄 License

Distributed under the Apache 2.0 License. See [`LICENSE`](LICENSE) for details.
