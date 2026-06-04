#!/usr/bin/env bash
#
# cluster-up.sh — bring up the full cfd-platform experiment cluster on a
# Vast.ai Ubuntu 22.04 KVM VM (run as root, from the repo root).
#
# WHAT IT DOES (idempotent, re-runnable):
#   1. Host prep: swapfile, sysctl (inotify), Docker cgroup driver, shared
#      RWX host dirs under /mnt/cfd/{configs,results,graphs}.
#   2. Installs pinned kind v0.23.0 + kubectl v1.30.2.
#   3. Creates a kind cluster named "cfd" with 1 control-plane + 3*ZONE_SIZE
#      workers, every node bind-mounting /mnt/cfd/* at the same path.
#   4. Labels workers topology.kubernetes.io/zone=a|b|c and pins each node
#      container to a distinct CPU core (docker update --cpuset-cpus).
#   5. Applies tc netem one-way egress delays (RTT/2) between worker IPs.
#   6. Applies k8s manifests in dependency order: namespace -> RWX PV/PVC
#      (storageClassName cfd-shared) -> rbac -> MPI Operator v0.4.0 ->
#      named kube-scheduler "topology-aware-scheduler" -> extender (with a
#      regenerated latency.yaml from real node names) -> backend (built
#      locally, kind-loaded). Frontend (70-frontend.yaml) is NOT deployed.
#   7. Background port-forward 127.0.0.1:8080 -> svc/cfd-platform-backend:80.
#
# USAGE:
#   ZONE_SIZE=8   ./scripts/cluster-up.sh   # default: 24 workers (N=16 run)
#   ZONE_SIZE=12  ./scripts/cluster-up.sh   # 36 workers   (N=32 scaling run)
#
# PREREQUISITES:
#   - Repo present at /root/cfd-platform; run from the repo root.
#   - Run as root on a real KVM VM (needs CAP_NET_ADMIN for tc, sysctl rw,
#     cgroup v2 rw, Docker Engine already installed).
#   - GHCR solver/scheduler images are PUBLIC; the backend image is built
#     locally (no backend image exists on GHCR).
#
# Latency model (full round-trip ms): intra-zone 0.5; a<->b 5; b<->c 5;
# a<->c 10. tc netem applies RTT/2 one-way at EACH endpoint; latency.yaml
# stores the FULL RTT. Both are derived from the single rtt_ms()/ZONE[] core.
#
set -euo pipefail

# ---------------------------------------------------------------------------
# Fixed conventions / parameters (single source of truth)
# ---------------------------------------------------------------------------
ZONE_SIZE="${ZONE_SIZE:-8}"            # nodes per zone (default 8 -> 24 workers)
NUM_WORKERS=$(( 3 * ZONE_SIZE ))       # 24 default, 36 for the scaling run
CLUSTER_NAME=cfd
NAMESPACE=cfd-platform

KIND_VERSION=v0.23.0                    # ships kindest/node:v1.30.0
KUBECTL_VERSION=v1.30.2                 # matches cluster k8s minor (1.30)
KIND_NODE_IMAGE=kindest/node:v1.30.0    # pinned explicitly in kind config
KUBE_SCHEDULER_IMAGE=registry.k8s.io/kube-scheduler:v1.30.0  # MUST match minor

SWAP_FILE=/swapfile
SWAP_SIZE_MB=16384                      # 16 GB (disk is only ~118 GB free)

REPO_ROOT="$(pwd)"
K8S=/mnt/cfd                            # host shared-storage root

log() { echo "==> $*"; }

# ---------------------------------------------------------------------------
# Shared core helpers (used by BOTH tc netem and latency.yaml generation).
# Keeping these defined once is what guarantees tc one-way delays and the
# latency.yaml RTTs cannot diverge.
# ---------------------------------------------------------------------------

# zone_for_index <i>  -> a|b|c   (1-based worker index)
zone_for_index() {
  local i="$1"
  if   (( i <= ZONE_SIZE ));   then echo a
  elif (( i <= 2*ZONE_SIZE )); then echo b
  else                              echo c
  fi
}

# worker_container <i> -> kind container/node name.
# Off-by-one gotcha: index 1 == "cfd-worker" (no digit), index k>=2 == "cfd-workerk".
worker_container() {
  local i="$1"
  if (( i == 1 )); then echo "${CLUSTER_NAME}-worker"
  else                  echo "${CLUSTER_NAME}-worker${i}"; fi
}

# index_of_node cfd-worker -> 1 ; cfd-worker2 -> 2
index_of_node() {
  local n="${1#${CLUSTER_NAME}-worker}"   # "" for cfd-worker, "2".."N" otherwise
  if [[ -z "$n" ]]; then echo 1; else echo "$n"; fi
}

# rtt_ms <zoneX> <zoneY> -> full RTT in ms (string, may be decimal)
rtt_ms() {
  local x="$1" y="$2"
  if [[ "$x" == "$y" ]]; then echo "0.5"; return; fi
  case "${x}${y}" in
    ab|ba) echo "5"  ;;
    bc|cb) echo "5"  ;;
    ac|ca) echo "10" ;;
    *)     echo ""   ;;   # fall-through (e.g. an unzoned node) -> caught upstream
  esac
}

# oneway_ms <rtt> -> one-way egress delay in ms with 3 decimals (for tc netem)
oneway_ms() { awk -v r="$1" 'BEGIN{printf "%.3f", r/2}'; }

# node_ip <container> -> the container's IP on the "kind" docker network
node_ip() { docker inspect -f '{{ .NetworkSettings.Networks.kind.IPAddress }}' "$1"; }

# ===========================================================================
# STEP 0 — sanity checks
# ===========================================================================
log "STEP 0: preflight checks"
if [[ "$(id -u)" -ne 0 ]]; then
  echo "ERROR: must run as root" >&2; exit 1
fi
if ! command -v docker >/dev/null 2>&1; then
  echo "ERROR: docker not found (Docker Engine is a prerequisite)" >&2; exit 1
fi
NPROC="$(nproc)"
# control-plane pinned to cores 0-3; each worker gets 2 dedicated cores
# (4+2(k-1), 5+2(k-1)). Max core index = 3 + 2*NUM_WORKERS -> need that many vCPUs.
NEED_CORES=$(( 4 + 2 * NUM_WORKERS ))
if (( NEED_CORES > NPROC )); then
  echo "ERROR: need >= ${NEED_CORES} vCPUs for cpuset pinning (2 cores/worker), have ${NPROC}" >&2
  exit 1
fi

# Disk headroom: kind node images + local backend build + runtime image pulls
# (codeaster is multi-GB) + 16G swap + results. Fail early, not mid-run.
FREE_GB="$(df -BG --output=avail / | tail -1 | tr -dc '0-9')"
if (( FREE_GB < 40 )); then
  echo "ERROR: only ${FREE_GB}G free on / — need >= 40G (images + swap + results)" >&2
  exit 1
fi

# Registries reachable BEFORE committing paid VM hours (401 on /v2/ = reachable).
for url in https://ghcr.io/v2/ https://registry.k8s.io/v2/ https://registry-1.docker.io/v2/; do
  code="$(curl -s -o /dev/null -w '%{http_code}' -m 15 "$url" || echo 000)"
  if [[ "$code" == "000" ]]; then
    echo "ERROR: cannot reach ${url} (needed for image pulls)" >&2; exit 1
  fi
done

# netem kernel module (tc needs sch_netem; no-op if built in).
modprobe sch_netem 2>/dev/null || true

log "ZONE_SIZE=${ZONE_SIZE} -> NUM_WORKERS=${NUM_WORKERS}; vCPUs=${NPROC}; free=${FREE_GB}G"

# ===========================================================================
# STEP 1 — host prep: swap, sysctl, Docker cgroup driver, shared dirs
# ===========================================================================
log "STEP 1: host prep (swap, sysctl, docker cgroup driver, shared dirs)"

# 1a. 16 GB swapfile (no swap on the VM; do not overcommit on a 118 GB disk).
if ! swapon --show 2>/dev/null | grep -q "${SWAP_FILE}"; then
  if [[ ! -f "${SWAP_FILE}" ]]; then
    log "creating ${SWAP_SIZE_MB}MB swapfile at ${SWAP_FILE}"
    fallocate -l "${SWAP_SIZE_MB}M" "${SWAP_FILE}" 2>/dev/null \
      || dd if=/dev/zero of="${SWAP_FILE}" bs=1M count="${SWAP_SIZE_MB}" status=none
    chmod 600 "${SWAP_FILE}"
    mkswap "${SWAP_FILE}" >/dev/null
  fi
  swapon "${SWAP_FILE}"
  grep -q "^${SWAP_FILE} " /etc/fstab 2>/dev/null \
    || printf '%s none swap sw 0 0\n' "${SWAP_FILE}" >> /etc/fstab
else
  log "swap already active on ${SWAP_FILE}"
fi

# 1b. inotify limits — #1 kind-on-many-nodes failure if left at defaults.
sysctl -w fs.inotify.max_user_watches=1048576 >/dev/null
sysctl -w fs.inotify.max_user_instances=8192 >/dev/null
printf 'fs.inotify.max_user_watches=1048576\nfs.inotify.max_user_instances=8192\n' \
  > /etc/sysctl.d/99-kind.conf

# 1c. Docker must use the systemd cgroup driver (kind node images default to
#     systemd; a cgroupfs mismatch crashloops kubelets).
DOCKER_DRIVER="$(docker info -f '{{ .CgroupDriver }}' 2>/dev/null || echo unknown)"
if [[ "${DOCKER_DRIVER}" == "systemd" ]]; then
  log "Docker cgroup driver already systemd"
elif kind get clusters 2>/dev/null | grep -qx "${CLUSTER_NAME}"; then
  # Restarting dockerd would bounce/corrupt the already-running kind cluster.
  log "WARNING: Docker driver is '${DOCKER_DRIVER}' (not systemd) but cluster '${CLUSTER_NAME}' already exists — NOT restarting Docker."
else
  log "setting Docker cgroup driver to systemd (was: ${DOCKER_DRIVER})"
  mkdir -p /etc/docker
  if [[ -f /etc/docker/daemon.json ]]; then
    cp /etc/docker/daemon.json "/etc/docker/daemon.json.bak.$(date +%s)"
  fi
  cat > /etc/docker/daemon.json <<'JSON'
{
  "exec-opts": ["native.cgroupdriver=systemd"]
}
JSON
  systemctl restart docker
  # give the daemon a moment to come back (actually wait between probes)
  for _ in $(seq 1 30); do docker info >/dev/null 2>&1 && break; sleep 1; done
fi

# 1d. shared RWX host dirs (must exist BEFORE kind create — extraMounts wire
#     them in at node-container creation; non-root solver pods need 0777).
mkdir -p "${K8S}/configs" "${K8S}/results" "${K8S}/graphs"
chmod 0777 "${K8S}" "${K8S}/configs" "${K8S}/results" "${K8S}/graphs"

# ===========================================================================
# STEP 2 — install pinned kind + kubectl
# ===========================================================================
log "STEP 2: install pinned kind ${KIND_VERSION} + kubectl ${KUBECTL_VERSION}"
install -d /usr/local/bin

if ! command -v kind >/dev/null 2>&1 || [[ "$(kind version 2>/dev/null)" != *"${KIND_VERSION}"* ]]; then
  log "installing kind ${KIND_VERSION}"
  curl -fsSLo /usr/local/bin/kind \
    "https://kind.sigs.k8s.io/dl/${KIND_VERSION}/kind-linux-amd64"
  chmod +x /usr/local/bin/kind
else
  log "kind ${KIND_VERSION} already present"
fi

if ! command -v kubectl >/dev/null 2>&1 || [[ "$(kubectl version --client -o yaml 2>/dev/null | grep -o "${KUBECTL_VERSION}")" != "${KUBECTL_VERSION}" ]]; then
  log "installing kubectl ${KUBECTL_VERSION}"
  curl -fsSLo /usr/local/bin/kubectl \
    "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl"
  chmod +x /usr/local/bin/kubectl
else
  log "kubectl ${KUBECTL_VERSION} already present"
fi

# ===========================================================================
# STEP 3 — create the kind cluster (idempotent)
# ===========================================================================
log "STEP 3: create kind cluster '${CLUSTER_NAME}' (1 control-plane + ${NUM_WORKERS} workers)"

EXTRA_MOUNTS_BLOCK='    extraMounts:
      - hostPath: /mnt/cfd/configs
        containerPath: /mnt/cfd/configs
      - hostPath: /mnt/cfd/results
        containerPath: /mnt/cfd/results
      - hostPath: /mnt/cfd/graphs
        containerPath: /mnt/cfd/graphs'

KCFG="${REPO_ROOT}/kind-config.yaml"
{
  cat <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
name: ${CLUSTER_NAME}
nodes:
  - role: control-plane
    image: ${KIND_NODE_IMAGE}
    # failSwapOn:false so kubelet tolerates the 16 GB host swapfile we added.
    kubeadmConfigPatches:
      - |
        kind: KubeletConfiguration
        cgroupDriver: systemd
        failSwapOn: false
${EXTRA_MOUNTS_BLOCK}
EOF
  for ((i=1; i<=NUM_WORKERS; i++)); do
    cat <<EOF
  - role: worker
    image: ${KIND_NODE_IMAGE}
    kubeadmConfigPatches:
      - |
        kind: KubeletConfiguration
        cgroupDriver: systemd
        failSwapOn: false
${EXTRA_MOUNTS_BLOCK}
EOF
  done
} > "${KCFG}"

if kind get clusters 2>/dev/null | grep -qx "${CLUSTER_NAME}"; then
  log "cluster '${CLUSTER_NAME}' already exists — skipping create"
else
  kind create cluster --config "${KCFG}" --wait 300s
fi

# kubeconfig context
kubectl cluster-info --context "kind-${CLUSTER_NAME}" >/dev/null
kubectl config use-context "kind-${CLUSTER_NAME}" >/dev/null

log "waiting for ALL nodes Ready (--wait only covers control-plane)"
kubectl wait --for=condition=Ready nodes --all --timeout=600s

# ===========================================================================
# STEP 4 — zone labels (post-create, idempotent)
# ===========================================================================
log "STEP 4: label workers with topology.kubernetes.io/zone"
for ((i=1; i<=NUM_WORKERS; i++)); do
  node="$(worker_container "$i")"
  zone="$(zone_for_index "$i")"
  kubectl label node "${node}" "topology.kubernetes.io/zone=${zone}" --overwrite
done

# ===========================================================================
# STEP 5 — cpuset-pin node containers (AFTER Ready, so kubeadm init isn't
#          starved of CPU). control-plane -> 0-3; worker k -> core (3 + k).
# ===========================================================================
log "STEP 5: cpuset-pin node containers (control-plane 0-3; 2 dedicated cores/worker)"
# 2 cores/worker (not 1): MPI worker pods request cpu:"1" plus kindnet/kube-proxy
# DaemonSets request a little; if cadvisor derives node capacity from the cpuset,
# a 1-core node would report allocatable < 1000m and every worker pod would stay
# Pending. 2 cores keeps allocatable >= ~1900m. The rank still gets dedicated CPU
# (Burstable QoS, no limit) and clean isolation — just on a 2-core island.
docker update --cpuset-cpus "0-3" "${CLUSTER_NAME}-control-plane"
for ((k=1; k<=NUM_WORKERS; k++)); do
  c="$(worker_container "$k")"
  lo=$(( 4 + 2 * (k - 1) )); hi=$(( lo + 1 ))
  docker update --cpuset-cpus "${lo}-${hi}" "${c}"
done

# Assert the MPI worker pods (request cpu:"1") will actually fit, else fail fast
# instead of half-deploying and discovering Pending pods later on paid hours.
cpu_to_milli() { local v="$1"; if [[ "$v" == *m ]]; then echo "${v%m}"; else echo $(( ${v%.*} * 1000 )); fi; }
sample_worker="$(worker_container 1)"
alloc="$(kubectl get node "${sample_worker}" -o jsonpath='{.status.allocatable.cpu}')"
if (( $(cpu_to_milli "${alloc}") < 1000 )); then
  echo "ERROR: worker ${sample_worker} allocatable cpu=${alloc} (<1000m); MPI worker pods request 1 core and would stay Pending." >&2
  echo "       Increase cores/worker, or set kubeReserved/systemReserved=0 in the STEP 3 kubelet patch." >&2
  exit 1
fi
log "worker allocatable cpu=${alloc} (>=1000m, MPI worker pods fit)"

# ===========================================================================
# STEP 6 — build shared name/zone/IP maps (single source of truth for both
#          tc netem and latency.yaml). Enumerate the ACTUAL worker containers.
# ===========================================================================
log "STEP 6: discover worker node names, zones, and kind-network IPs"
mapfile -t NODES < <(docker ps --format '{{.Names}}' \
  | grep -E "^${CLUSTER_NAME}-worker[0-9]*$" \
  | while read -r n; do printf '%s %s\n' "$(index_of_node "$n")" "$n"; done \
  | sort -n | awk '{print $2}')

if (( ${#NODES[@]} != NUM_WORKERS )); then
  echo "ERROR: expected ${NUM_WORKERS} worker containers, found ${#NODES[@]}" >&2
  exit 1
fi

declare -A ZONE IP PODCIDR
for n in "${NODES[@]}"; do
  i="$(index_of_node "$n")"
  ZONE["$n"]="$(zone_for_index "$i")"
  IP["$n"]="$(node_ip "$n")"
  # Per-node pod CIDR (kindnet gives each node a distinct /24, e.g. 10.244.k.0/24).
  # This — NOT the node IP — is the L3 dst of MPI rank<->rank traffic, so it is
  # what tc netem must match (see STEP 7).
  PODCIDR["$n"]="$(kubectl get node "$n" -o jsonpath='{.spec.podCIDR}')"
  if [[ -z "${IP[$n]}" ]]; then
    echo "ERROR: no kind-network IP for ${n}" >&2; exit 1
  fi
  if [[ -z "${PODCIDR[$n]}" ]]; then
    echo "ERROR: no .spec.podCIDR for ${n} (CNI not ready?)" >&2; exit 1
  fi
done

# ===========================================================================
# STEP 7 — tc netem multi-AZ latency (one-way egress = RTT/2 to peer IPs only)
# ===========================================================================
log "STEP 7: apply tc netem inter-worker latency"

# Preflight: confirm tc + netem are usable INSIDE a kind node (kindest/node ships
# iproute2, but fail loudly+early here rather than mid-loop under set -e).
probe_node="$(worker_container 1)"
if ! docker exec "${probe_node}" sh -c \
   'command -v tc >/dev/null && tc qdisc add dev eth0 root netem delay 1ms && tc qdisc del dev eth0 root' \
   >/dev/null 2>&1; then
  echo "ERROR: tc/netem not usable inside kind node ${probe_node} (iproute2 missing or sch_netem unavailable)" >&2
  exit 1
fi

apply_tc_for_node() {
  local n="$1" dev=eth0 thiszone="${ZONE[$n]}"

  # 0) idempotent: wipe any prior root qdisc.
  docker exec "$n" tc qdisc del dev "$dev" root 2>/dev/null || true

  # 1) classful HTB root; class 1:999 is the UNDELAYED default — host/API
  #    server/DNS/registry/extender traffic must NOT be delayed.
  docker exec "$n" tc qdisc add dev "$dev" root handle 1: htb default 999
  docker exec "$n" tc class add dev "$dev" parent 1: classid 1:999 htb rate 10gbit

  # 2) one HTB class + netem leaf per DISTINCT one-way delay needed from here.
  local -A want=()
  local p d
  for p in "${NODES[@]}"; do
    [[ "$p" == "$n" ]] && continue
    d="$(oneway_ms "$(rtt_ms "$thiszone" "${ZONE[$p]}")")"
    want["$d"]=1
  done

  local -A cid=()
  local k=10
  for d in "${!want[@]}"; do
    cid["$d"]="$k"
    docker exec "$n" tc class add dev "$dev" parent 1: classid "1:${k}" htb rate 10gbit
    # jitter 0 (deterministic experiment); limit 100000 so high-BDP MPI bursts
    # are not dropped by netem's default 1000-packet backlog.
    docker exec "$n" tc qdisc add dev "$dev" parent "1:${k}" handle "${k}0:" \
      netem delay "${d}ms" limit 100000
    k=$(( k + 10 ))
  done

  # 3) u32 dst filters per peer -> the class matching its one-way delay.
  #    Match BOTH the peer's POD CIDR (where MPI rank<->rank and launcher<->worker
  #    SSH traffic is actually addressed — kindnet keeps the POD IP as the L3 dst,
  #    the node IP is only the L2 next hop) AND the peer NODE /32 (node-to-node).
  #    Matching only the node IP would miss ALL pod traffic and silently apply
  #    ZERO latency — the experiment's independent variable would be absent.
  for p in "${NODES[@]}"; do
    [[ "$p" == "$n" ]] && continue
    d="$(oneway_ms "$(rtt_ms "$thiszone" "${ZONE[$p]}")")"
    docker exec "$n" tc filter add dev "$dev" protocol ip parent 1: prio 1 \
      u32 match ip dst "${PODCIDR[$p]}" flowid "1:${cid[$d]}"
    docker exec "$n" tc filter add dev "$dev" protocol ip parent 1: prio 1 \
      u32 match ip dst "${IP[$p]}/32" flowid "1:${cid[$d]}"
  done
}

for n in "${NODES[@]}"; do
  apply_tc_for_node "$n"
done

# ===========================================================================
# STEP 8 — k8s manifests, dependency order. Render patched copies to /tmp so
#          the repo files stay clean and re-runs are idempotent.
# ===========================================================================

# --- 8a. namespace -------------------------------------------------------
log "STEP 8a: namespace"
kubectl apply -f "${REPO_ROOT}/k8s/00-namespace.yaml"

# --- 8b. RWX storage: cfd-shared hostPath PVs + PVCs pinned to them -------
# kind's default local-path SC is RWO/node-local and will NOT satisfy RWX.
# We create dummy-SC ("cfd-shared", no provisioner) hostPath PVs over the
# extraMount-ed dirs and bind the 3 PVCs to them with claimRef + volumeName.
log "STEP 8b: RWX storage (cfd-shared PVs + PVCs)"

PV_FILE=/tmp/cfd-shared-pvs.yaml
cat > "${PV_FILE}" <<'YAML'
---
apiVersion: v1
kind: PersistentVolume
metadata:
  name: cfd-configs-pv
spec:
  capacity: {storage: 20Gi}
  accessModes: [ReadWriteMany]
  persistentVolumeReclaimPolicy: Retain
  storageClassName: cfd-shared
  hostPath: {path: /mnt/cfd/configs, type: DirectoryOrCreate}
  claimRef: {namespace: cfd-platform, name: simulation-configs}
---
apiVersion: v1
kind: PersistentVolume
metadata:
  name: cfd-results-pv
spec:
  capacity: {storage: 50Gi}
  accessModes: [ReadWriteMany]
  persistentVolumeReclaimPolicy: Retain
  storageClassName: cfd-shared
  hostPath: {path: /mnt/cfd/results, type: DirectoryOrCreate}
  claimRef: {namespace: cfd-platform, name: simulation-results}
---
apiVersion: v1
kind: PersistentVolume
metadata:
  name: cfd-graphs-pv
spec:
  capacity: {storage: 1Gi}
  accessModes: [ReadWriteMany]
  persistentVolumeReclaimPolicy: Retain
  storageClassName: cfd-shared
  hostPath: {path: /mnt/cfd/graphs, type: DirectoryOrCreate}
  claimRef: {namespace: cfd-platform, name: scheduler-graphs}
YAML

# Retain PVs go "Released" (not Available) after a PVC delete, so re-runs
# won't rebind. Recreate the PVs each run (host data under /mnt/cfd is
# untouched) for deterministic idempotent binding.
kubectl delete pv cfd-configs-pv cfd-results-pv cfd-graphs-pv --ignore-not-found
kubectl apply -f "${PV_FILE}"

PVC_FILE=/tmp/cfd-10-storage.rendered.yaml
cat > "${PVC_FILE}" <<'YAML'
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata: {name: simulation-configs, namespace: cfd-platform}
spec:
  accessModes: [ReadWriteMany]
  storageClassName: cfd-shared
  volumeName: cfd-configs-pv
  resources: {requests: {storage: 20Gi}}
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata: {name: simulation-results, namespace: cfd-platform}
spec:
  accessModes: [ReadWriteMany]
  storageClassName: cfd-shared
  volumeName: cfd-results-pv
  resources: {requests: {storage: 50Gi}}
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata: {name: scheduler-graphs, namespace: cfd-platform}
spec:
  accessModes: [ReadWriteMany]
  storageClassName: cfd-shared
  volumeName: cfd-graphs-pv
  resources: {requests: {storage: 1Gi}}
YAML
# NOTE: do NOT apply k8s/10-storage.yaml unpatched — it has no storageClassName
# and would bind to kind's RWO default SC (immutable, cannot be fixed later).
kubectl apply -f "${PVC_FILE}"

# --- 8c. rbac (backend SA/Role/RoleBinding) ------------------------------
log "STEP 8c: backend RBAC"
kubectl apply -f "${REPO_ROOT}/k8s/20-rbac.yaml"

# --- 8d. MPI Operator v0.4.0 ---------------------------------------------
log "STEP 8d: MPI Operator v0.4.0"
kubectl apply -f https://raw.githubusercontent.com/kubeflow/mpi-operator/v0.4.0/deploy/v2beta1/mpi-operator.yaml
log "waiting for MPIJob CRD Established"
kubectl wait --for=condition=Established --timeout=180s crd/mpijobs.kubeflow.org
log "waiting for mpi-operator controller rollout"
kubectl -n mpi-operator rollout status deployment/mpi-operator --timeout=240s

# --- 8e. named kube-scheduler "topology-aware-scheduler" ------------------
# DECISION: a SEPARATE kube-scheduler Deployment (NOT kubeadmConfigPatches on
# the control-plane scheduler). Rationale:
#   * Default kube-scheduler stays untouched -> all system/bootstrap pods keep
#     scheduling; no risk of bricking bring-up with a broken extender URL.
#   * Only pods that explicitly set schedulerName: topology-aware-scheduler are
#     handled by it; everything else uses default-scheduler.
#   * Pure `kubectl apply` -> idempotent / re-runnable, no kind-create mutation.
# The KubeSchedulerConfiguration here OVERRIDES the repo's 50-scheduler-config.yaml,
# whose extenders block uses `managedResources: topology.scheduler/mpi-job`
# (gates the extender on a resource MPIJob pods never request -> extender never
# fires). We drop managedResources and set ignorable:true so non-MPI pods are
# unaffected and bootstrap proceeds even if the extender pod isn't Ready.
#
# NAMING: the EXTENDER Deployment+Service is "topology-aware-scheduler"
# (k8s/40). This kube-scheduler Deployment is "topology-aware-kube-scheduler"
# to avoid a collision. The PROFILE name (schedulerName) is the string
# "topology-aware-scheduler" — independent of any object name.
log "STEP 8e: deploy named kube-scheduler topology-aware-scheduler"
KSCHED_FILE=/tmp/topology-aware-kube-scheduler.yaml
cat > "${KSCHED_FILE}" <<YAML
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: topology-aware-scheduler-sa
  namespace: cfd-platform
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: topology-aware-scheduler-as-kube-scheduler
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: system:kube-scheduler
subjects:
  - kind: ServiceAccount
    name: topology-aware-scheduler-sa
    namespace: cfd-platform
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: topology-aware-scheduler-volume-binder
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: system:volume-scheduler
subjects:
  - kind: ServiceAccount
    name: topology-aware-scheduler-sa
    namespace: cfd-platform
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: topology-aware-scheduler-config-ksc
  namespace: cfd-platform
data:
  scheduler-config.yaml: |
    apiVersion: kubescheduler.config.k8s.io/v1
    kind: KubeSchedulerConfiguration
    leaderElection:
      leaderElect: false
    profiles:
      - schedulerName: topology-aware-scheduler
        plugins:
          multiPoint:
            enabled:
              - name: PrioritySort
              - name: NodeResourcesFit
              - name: NodeAffinity
              - name: PodTopologySpread
    extenders:
      - urlPrefix: "http://topology-aware-scheduler.cfd-platform.svc.cluster.local"
        prioritizeVerb: "prioritize"
        weight: 5
        enableHTTPS: false
        nodeCacheCapable: false
        ignorable: true
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: topology-aware-kube-scheduler
  namespace: cfd-platform
  labels: {app: topology-aware-kube-scheduler}
spec:
  replicas: 1
  selector:
    matchLabels: {app: topology-aware-kube-scheduler}
  template:
    metadata:
      labels: {app: topology-aware-kube-scheduler}
    spec:
      serviceAccountName: topology-aware-scheduler-sa
      containers:
        - name: kube-scheduler
          image: ${KUBE_SCHEDULER_IMAGE}
          command:
            - kube-scheduler
            - --config=/etc/kubernetes/scheduler-config.yaml
            - --leader-elect=false
            - --v=2
          volumeMounts:
            - name: config
              mountPath: /etc/kubernetes
              readOnly: true
          resources:
            requests: {cpu: 100m, memory: 128Mi}
            limits:   {cpu: 1,    memory: 512Mi}
      volumes:
        - name: config
          configMap:
            name: topology-aware-scheduler-config-ksc
YAML
kubectl apply -f "${KSCHED_FILE}"
# This is the ONLY scheduler that services MPIJob pods (schedulerName=
# topology-aware-scheduler). If it never goes Ready, every MPIJob hangs Pending
# while the script would otherwise print DONE — wait + fail loudly.
log "waiting for topology-aware-kube-scheduler rollout"
kubectl -n "${NAMESPACE}" rollout status deployment/topology-aware-kube-scheduler --timeout=180s

# --- 8f. extender + regenerated latency.yaml -----------------------------
# Apply 40 first (creates Deployment/Service + the STALE 9-node ConfigMap),
# then regenerate latency.yaml from real node names, recreate the ConfigMap,
# and rollout-restart the extender (subPath CM mounts do NOT auto-update; the
# extender loads latency only at startup).
log "STEP 8f: extender (40-scheduler-extender.yaml) + regenerated latency.yaml"
kubectl apply -f "${REPO_ROOT}/k8s/40-scheduler-extender.yaml"

LAT_FILE=/tmp/latency.yaml
{
  echo "# generated by cluster-up.sh — full RTT ms, keyed by actual kind node names"
  echo "# zone map: ZONE_SIZE=${ZONE_SIZE}  a=workers 1..${ZONE_SIZE}  b=..$((2*ZONE_SIZE))  c=rest"
  cnt=${#NODES[@]}
  for ((x=0; x<cnt; x++)); do
    for ((y=x+1; y<cnt; y++)); do
      a="${NODES[x]}"; b="${NODES[y]}"
      printf '%s %s %s\n' "$a" "$b" "$(rtt_ms "${ZONE[$a]}" "${ZONE[$b]}")"
    done
  done
} > "${LAT_FILE}"

# Validate: every non-comment line must have EXACTLY 3 fields, else loadLatency
# rejects the WHOLE file and the extender runs with an empty matrix (silent).
if ! awk 'NF && $1 !~ /^#/ { if (NF != 3) { print NR": "$0 > "/dev/stderr"; exit 1 } }' "${LAT_FILE}"; then
  echo "ERROR: generated latency.yaml has a malformed line" >&2; exit 1
fi

kubectl -n "${NAMESPACE}" create configmap topology-aware-scheduler-config \
  --from-file=latency.yaml="${LAT_FILE}" \
  --dry-run=client -o yaml | kubectl apply -f -
kubectl -n "${NAMESPACE}" rollout restart deployment/topology-aware-scheduler
kubectl -n "${NAMESPACE}" rollout status  deployment/topology-aware-scheduler --timeout=180s

# --- 8g. backend: build locally, kind-load, deploy with Never pull --------
# Build context is backend/ (Dockerfile does `COPY go.mod go.sum ./`, and
# go.mod lives in backend/). kind load into cluster "cfd". Pre-render the
# manifest (image + imagePullPolicy: Never) to avoid a transient ImagePullBackOff
# (GHCR has no backend image). Solver/scheduler images are pulled from public
# GHCR at pod runtime (IfNotPresent) — NOT kind-loaded (codeaster is multi-GB;
# loading into every node would blow the 118 GB disk).
log "STEP 8g: build + load + deploy backend (cfd-platform-backend:local)"
docker build -t cfd-platform-backend:local -f "${REPO_ROOT}/backend/Dockerfile" "${REPO_ROOT}/backend"
kind load docker-image cfd-platform-backend:local --name "${CLUSTER_NAME}"

BACKEND_FILE=/tmp/60-backend.rendered.yaml
sed -e 's#image: ghcr.io/theweirdfulmurk/cfd-platform-backend:latest#image: cfd-platform-backend:local\n          imagePullPolicy: Never#' \
  "${REPO_ROOT}/k8s/60-backend.yaml" > "${BACKEND_FILE}"
# Assert the patch landed — a silent sed no-op would leave the nonexistent
# :latest GHCR ref and the backend would ImagePullBackOff.
if ! grep -q 'image: cfd-platform-backend:local' "${BACKEND_FILE}" \
   || ! grep -q 'imagePullPolicy: Never' "${BACKEND_FILE}"; then
  echo "ERROR: backend image patch did not apply (k8s/60-backend.yaml format changed?)" >&2
  exit 1
fi
kubectl apply -f "${BACKEND_FILE}"

# NOTE: frontend (k8s/70-frontend.yaml) is intentionally NOT deployed — the
# Python orchestrator drives the backend directly.

log "waiting for backend rollout"
kubectl -n "${NAMESPACE}" rollout status deployment/cfd-platform-backend --timeout=240s

# ===========================================================================
# STEP 9 — RWX cross-node verification (write on a zone-c worker, read on CP)
# ===========================================================================
log "STEP 9: verify RWX cross-node visibility"
kubectl -n "${NAMESPACE}" wait --for=jsonpath='{.status.phase}'=Bound \
  pvc/simulation-results --timeout=120s

TOKEN="rwx-$(date +%s)"
kubectl -n "${NAMESPACE}" delete pod rwx-writer --ignore-not-found >/dev/null 2>&1 || true
kubectl -n "${NAMESPACE}" run rwx-writer --restart=Never --image=busybox \
  --overrides='{"spec":{"nodeSelector":{"topology.kubernetes.io/zone":"c"},
    "containers":[{"name":"c","image":"busybox","command":["sh","-c","echo '"${TOKEN}"' > /results/.rwxcheck && sleep 5"],
    "volumeMounts":[{"name":"r","mountPath":"/results"}]}],
    "volumes":[{"name":"r","persistentVolumeClaim":{"claimName":"simulation-results"}}]}}' \
  --command -- sh >/dev/null
kubectl -n "${NAMESPACE}" wait --for=condition=Ready pod/rwx-writer --timeout=120s || true
sleep 3

READ="$(kubectl -n "${NAMESPACE}" run rwx-reader --restart=Never --rm -i --image=busybox \
  --overrides='{"spec":{"nodeName":"'"${CLUSTER_NAME}"'-control-plane","tolerations":[{"operator":"Exists"}],
    "containers":[{"name":"c","image":"busybox","stdin":true,"command":["sh","-c","cat /results/.rwxcheck"],
    "volumeMounts":[{"name":"r","mountPath":"/results"}]}],
    "volumes":[{"name":"r","persistentVolumeClaim":{"claimName":"simulation-results"}}]}}' \
  --command -- sh 2>/dev/null | tr -d '\r\n ')" || true
kubectl -n "${NAMESPACE}" delete pod rwx-writer --ignore-not-found >/dev/null 2>&1 || true

kubectl -n "${NAMESPACE}" delete pod rwx-reader --ignore-not-found >/dev/null 2>&1 || true
if [[ "${READ}" == "${TOKEN}" ]]; then
  log "RWX OK: token '${TOKEN}' written on a zone-c worker, read on control-plane"
else
  echo "!! RWX verification inconclusive: expected '${TOKEN}', got '${READ}'" >&2
  echo "   (continuing; check /mnt/cfd extraMounts + PVC binding if this persists)" >&2
fi

# ===========================================================================
# STEP 9b — functional scheduler probe: a pod with
#   schedulerName=topology-aware-scheduler MUST be scheduled by our named
#   kube-scheduler. If not, every MPIJob hangs Pending forever (silent waste).
# ===========================================================================
log "STEP 9b: verify topology-aware-scheduler schedules pods"
kubectl -n "${NAMESPACE}" delete pod sched-probe --ignore-not-found >/dev/null 2>&1 || true
kubectl -n "${NAMESPACE}" run sched-probe --restart=Never --image=registry.k8s.io/pause:3.9 \
  --overrides='{"spec":{"schedulerName":"topology-aware-scheduler"}}' >/dev/null
if kubectl -n "${NAMESPACE}" wait --for=jsonpath='{.status.phase}'=Running pod/sched-probe --timeout=90s >/dev/null 2>&1; then
  log "scheduler probe Running — topology-aware-scheduler works"
else
  echo "ERROR: pod under schedulerName=topology-aware-scheduler did not run; the named kube-scheduler is not scheduling (MPIJobs would hang Pending)." >&2
  kubectl -n "${NAMESPACE}" describe pod sched-probe 2>/dev/null | tail -25 >&2
  kubectl -n "${NAMESPACE}" logs deploy/topology-aware-kube-scheduler --tail=25 2>/dev/null >&2 || true
  exit 1
fi
kubectl -n "${NAMESPACE}" delete pod sched-probe --ignore-not-found >/dev/null 2>&1 || true

# ===========================================================================
# STEP 9c — verify the multi-AZ latency contrast is ACTUALLY applied to POD
#   traffic. This is the experiment's independent variable; a silent miss
#   (e.g. tc matching node IPs instead of pod CIDRs) invalidates every run.
#   Two pods in zone-a / zone-c, ping pod->pod, assert RTT ~ 10ms (not ~0).
# ===========================================================================
log "STEP 9c: verify cross-zone pod-to-pod RTT (~10 ms a<->c)"
kubectl -n "${NAMESPACE}" delete pod rtt-a rtt-c --ignore-not-found >/dev/null 2>&1 || true
for zc in a:rtt-a c:rtt-c; do
  z="${zc%%:*}"; nm="${zc##*:}"
  kubectl -n "${NAMESPACE}" run "${nm}" --restart=Never --image=busybox \
    --overrides='{"spec":{"nodeSelector":{"topology.kubernetes.io/zone":"'"${z}"'"},"containers":[{"name":"c","image":"busybox","command":["sh","-c","sleep 180"]}]}}' >/dev/null
done
if kubectl -n "${NAMESPACE}" wait --for=condition=Ready pod/rtt-a pod/rtt-c --timeout=120s >/dev/null 2>&1; then
  cip="$(kubectl -n "${NAMESPACE}" get pod rtt-c -o jsonpath='{.status.podIP}')"
  rtt="$(kubectl -n "${NAMESPACE}" exec rtt-a -- ping -c 5 -q "${cip}" 2>/dev/null \
        | awk -F'/' '/round-trip|rtt/{print $5}')"
  if [[ -n "${rtt}" ]]; then
    log "measured a<->c pod RTT = ${rtt} ms (modelled ~10)"
    if awk -v r="${rtt}" 'BEGIN{ exit !(r < 6) }'; then
      echo "ERROR: cross-zone pod RTT=${rtt}ms is far below the modelled 10ms — tc netem is NOT shaping pod traffic; the experiment would be invalid. Check STEP 7 podCIDR filters (docker exec ${probe_node} tc -s filter show dev eth0 parent 1:)." >&2
      exit 1
    fi
  else
    echo "!! could not parse pod RTT (ping output empty) — verify tc manually before running the experiment" >&2
  fi
else
  echo "!! rtt probe pods not Ready — skipping RTT check; verify tc manually before the experiment" >&2
fi
kubectl -n "${NAMESPACE}" delete pod rtt-a rtt-c --ignore-not-found >/dev/null 2>&1 || true

# ===========================================================================
# STEP 10 — background port-forward to the backend (idempotent)
# ===========================================================================
log "STEP 10: background port-forward 127.0.0.1:8080 -> svc/cfd-platform-backend:80"
pkill -f 'port-forward.*cfd-platform-backend' 2>/dev/null || true
nohup kubectl -n "${NAMESPACE}" port-forward --address 127.0.0.1 \
  svc/cfd-platform-backend 8080:80 \
  > /var/log/cfd-backend-portforward.log 2>&1 &
echo "$!" > /var/run/cfd-portforward.pid 2>/dev/null || true

# Verify the tunnel target actually serves before claiming DONE (backend exposes
# GET /health -> "OK"); otherwise the orchestrator's first request gets refused.
pf_ok=
for _ in $(seq 1 30); do
  if curl -fsS -o /dev/null http://127.0.0.1:8080/health 2>/dev/null; then pf_ok=1; break; fi
  sleep 1
done
if [[ -n "${pf_ok}" ]]; then
  log "backend reachable at http://127.0.0.1:8080/health"
else
  echo "!! port-forward not reachable on 127.0.0.1:8080 after 30s — see /var/log/cfd-backend-portforward.log" >&2
fi

log "DONE. Cluster '${CLUSTER_NAME}' up: 1 control-plane + ${NUM_WORKERS} workers (ZONE_SIZE=${ZONE_SIZE})."
log "Backend reachable at http://127.0.0.1:8080 on the VM (SSH -L 8080:localhost:8080 to reach it)."
