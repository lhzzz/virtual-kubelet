package zhst

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"strings"
	"time"

	"github.com/virtual-kubelet/virtual-kubelet/cmd/virtual-kubelet/internal/provider/zhst/edge-proto/pb"
	"github.com/virtual-kubelet/virtual-kubelet/errdefs"
	"github.com/virtual-kubelet/virtual-kubelet/internal/manager"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	"github.com/virtual-kubelet/virtual-kubelet/node/api"
	stats "github.com/virtual-kubelet/virtual-kubelet/node/api/statsv1alpha1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// Provider configuration defaults.
	defaultCPUCapacity    = "20"
	defaultMemoryCapacity = "100Gi"
	defaultPodCapacity    = "20"

	// Values used in tracing as attribute keys.
	namespaceKey     = "namespace"
	nameKey          = "name"
	containerNameKey = "containerName"
)

var (
	netErrorPodStatus *v1.PodStatus = &v1.PodStatus{Phase: v1.PodUnknown, Reason: "ConnectProviderFailed"}
)

type ZhstConfig struct { // nolint:golint
	CPU         string `json:"cpu,omitempty"`
	Memory      string `json:"memory,omitempty"`
	Pods        string `json:"pods,omitempty"`
	EdgeAddress string `json:"edgeaddress,omitempty"`
}

type ZhstProvider struct { // nolint:golint
	nodeName           string
	operatingSystem    string
	internalIP         string
	daemonEndpointPort int32
	config             ZhstConfig
	startTime          time.Time
	notifier           func(*v1.Pod)
	edgeConnect        *grpc.ClientConn
	ignorePods         map[string]*v1.Pod
	nodeNotifer        func(*v1.Node)
	resourceManager    *manager.ResourceManager
}

func (zp *ZhstProvider) getEdgeletClient() (pb.EdgeletClient, error) {
	if zp.edgeConnect != nil {
		return pb.NewEdgeletClient(zp.edgeConnect), nil
	}
	if len(zp.config.EdgeAddress) == 0 {
		return nil, errors.New("edge address is empty")
	}
	edgeconn, err := grpc.Dial(zp.config.EdgeAddress, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	zp.edgeConnect = edgeconn
	return pb.NewEdgeletClient(edgeconn), nil
}

func NewZhstProvider(providerConfig, nodeName, operatingSystem, internalIP string, daemonEndpointPort int32, rsmgr *manager.ResourceManager) (*ZhstProvider, error) {
	config, err := loadConfig(providerConfig, nodeName)
	if err != nil {
		return nil, err
	}
	zp := &ZhstProvider{
		config:             config,
		nodeName:           nodeName,
		operatingSystem:    operatingSystem,
		internalIP:         internalIP,
		daemonEndpointPort: daemonEndpointPort,
		startTime:          time.Now(),
		ignorePods:         make(map[string]*v1.Pod),
		resourceManager:    rsmgr,
	}
	return zp, nil
}

// loadConfig loads the given json configuration files.
func loadConfig(providerConfig, nodeName string) (config ZhstConfig, err error) {
	log.L.Info("loadConfig, path:", providerConfig)
	data, err := ioutil.ReadFile(providerConfig)
	if err != nil {
		return config, err
	}
	//configMap := map[string]ZhstConfig{}
	err = json.Unmarshal(data, &config)
	if err != nil {
		return config, err
	}
	// if _, exist := configMap[nodeName]; exist {
	// 	config = configMap[nodeName]
	// }

	if config.CPU == "" {
		config.CPU = defaultCPUCapacity
	}
	if config.Memory == "" {
		config.Memory = defaultMemoryCapacity
	}
	if config.Pods == "" {
		config.Pods = defaultPodCapacity
	}

	if _, err = resource.ParseQuantity(config.CPU); err != nil {
		return config, fmt.Errorf("Invalid CPU value %v", config.CPU)
	}
	if _, err = resource.ParseQuantity(config.Memory); err != nil {
		return config, fmt.Errorf("Invalid memory value %v", config.Memory)
	}
	if _, err = resource.ParseQuantity(config.Pods); err != nil {
		return config, fmt.Errorf("Invalid pods value %v", config.Pods)
	}
	return config, nil
}

func (zp *ZhstProvider) CreatePod(ctx context.Context, pod *v1.Pod) (err error) {
	defer func() {
		if err == nil {
			zp.notifier(pod)
		}
	}()
	defaultPodStatus(pod)
	if isNeedIgnore(pod) {
		zp.ignorePods[pod.ObjectMeta.Name] = pod
		return nil
	}
	client, err := zp.getEdgeletClient()
	if err != nil {
		return err
	}
	ctx = metadata.AppendToOutgoingContext(ctx, "node", zp.nodeName)
	resp, err := client.CreatePod(ctx, &pb.CreatePodRequest{
		Pod: pod,
	})
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return fmt.Errorf(resp.Error.Msg)
	}
	pod = resp.Pod
	return nil
}

func (zp *ZhstProvider) UpdatePod(ctx context.Context, pod *v1.Pod) (err error) {
	defer func() {
		if err == nil {
			zp.notifier(pod)
		}
	}()
	if isNeedIgnore(pod) {
		zp.ignorePods[pod.ObjectMeta.Name] = pod
		return nil
	}
	client, err := zp.getEdgeletClient()
	if err != nil {
		return err
	}
	ctx = metadata.AppendToOutgoingContext(ctx, "node", zp.nodeName)
	resp, err := client.UpdatePod(ctx, &pb.UpdatePodRequest{
		Pod: pod,
	})
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return fmt.Errorf(resp.Error.Msg)
	}
	pod = resp.Pod
	return nil
}

func (zp *ZhstProvider) DeletePod(ctx context.Context, pod *v1.Pod) (err error) {
	defer func() {
		if err == nil {
			zp.notifier(pod)
		}
	}()

	prepareDeletePod(pod)
	if isNeedIgnore(pod) {
		delete(zp.ignorePods, pod.Name)
		return nil
	}
	client, err := zp.getEdgeletClient()
	if err != nil {
		return err
	}
	ctx = metadata.AppendToOutgoingContext(ctx, "node", zp.nodeName)
	resp, err := client.DeletePod(ctx, &pb.DeletePodRequest{
		Pod: pod,
	})
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return fmt.Errorf(resp.Error.Msg)
	}
	return nil
}

func (zp *ZhstProvider) GetPod(ctx context.Context, namespace, name string) (pod *v1.Pod, err error) {
	if ignorePod, ok := zp.ignorePods[name]; ok {
		pod = ignorePod
		return pod, nil
	}

	client, err := zp.getEdgeletClient()
	if err != nil {
		return nil, err
	}
	ctx = metadata.AppendToOutgoingContext(ctx, "node", zp.nodeName)
	resp, err := client.GetPod(ctx, &pb.GetPodRequest{
		Namespace: namespace,
		Name:      name,
	})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		if resp.Error.Code == pb.ErrorCode_NO_RESULT {
			return nil, errdefs.NotFoundf("pod \"%s/%s\" is not known to the provider", namespace, name)
		}
		return nil, fmt.Errorf(resp.Error.Msg)
	}
	pod = resp.Pod
	log.G(ctx).Info("GetPod name:", name, " , phase:", pod.Status.Phase, " reason:", pod.Status.Reason)
	return pod, nil
}

func (zp *ZhstProvider) GetContainerLogs(ctx context.Context, namespace, podName, containerName string, opts api.ContainerLogOpts) (io.ReadCloser, error) {
	return ioutil.NopCloser(strings.NewReader("")), nil
}

func (zp *ZhstProvider) RunInContainer(ctx context.Context, namespace, name, container string, cmd []string, attach api.AttachIO) error {
	return nil
}

func (zp *ZhstProvider) GetPodStatus(ctx context.Context, namespace, name string) (*v1.PodStatus, error) {
	ignorePod, ok := zp.ignorePods[name]
	if ok {
		return &ignorePod.Status, nil
	}

	client, err := zp.getEdgeletClient()
	if err != nil {
		log.G(ctx).Error("GetPodStatus getEdgeletClient failed,err=", err)
		return netErrorPodStatus, nil
	}
	ctx = metadata.AppendToOutgoingContext(ctx, "node", zp.nodeName)
	resp, err := client.GetPod(ctx, &pb.GetPodRequest{
		Namespace: namespace,
		Name:      name,
	})
	if err != nil {
		log.G(ctx).Error("ZhstProvider GetPodStatus failed,err=", err)
		return netErrorPodStatus, nil
	}
	if resp.Error != nil {
		if resp.Error.Code == pb.ErrorCode_NO_RESULT {
			return nil, errdefs.NotFoundf("pod \"%s/%s\" is not known to the provider", namespace, name)
		}
		return nil, fmt.Errorf(resp.Error.Msg)
	}
	return &resp.Pod.Status, nil
}

func (zp *ZhstProvider) GetPods(ctx context.Context) ([]*v1.Pod, error) {
	var pods []*v1.Pod
	for _, pod := range zp.ignorePods {
		pods = append(pods, pod)
	}

	client, err := zp.getEdgeletClient()
	if err != nil {
		return nil, err
	}
	ctx = metadata.AppendToOutgoingContext(ctx, "node", zp.nodeName)
	resp, err := client.GetPods(ctx, &pb.GetPodsRequest{})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf(resp.Error.Msg)
	}
	pods = append(pods, resp.Pods...)
	return pods, nil
}

func (zp *ZhstProvider) ConfigureNode(ctx context.Context, n *v1.Node) { // nolint:golint
	n.Status.Capacity = zp.capacity()
	n.Status.Allocatable = zp.capacity()
	n.Status.Conditions = zp.nodeConditions()
	n.Status.Addresses = zp.nodeAddresses()
	n.Status.DaemonEndpoints = zp.nodeDaemonEndpoints()
	os := zp.operatingSystem
	if os == "" {
		os = "linux"
	}
	n.Status.NodeInfo.OperatingSystem = os
	n.Status.NodeInfo.Architecture = "amd64"
	n.ObjectMeta.Labels["alpha.service-controller.kubernetes.io/exclude-balancer"] = "true"
	n.ObjectMeta.Labels["node.kubernetes.io/exclude-from-external-load-balancers"] = "true"
}

func (zp *ZhstProvider) GetStatsSummary(ctx context.Context) (*stats.Summary, error) {
	return nil, nil
}

func (zp *ZhstProvider) NotifyPods(ctx context.Context, notifier func(*v1.Pod)) {
	zp.notifier = notifier
}

//1、依靠ping每次去获取与edge的连接是否有问题，同时获取一下edge端的node参数：然后调用nodeNotify修改node是否ready
//2、初始化node ready的时机， 初始化后检测与边缘是否连接上了， 断开就notReady， 连上就ready （缓存一个标志位比较
func (zp *ZhstProvider) Ping(ctx context.Context) error {

	return ctx.Err()
}

// NotifyNodeStatus should not block callers.
func (zp *ZhstProvider) NotifyNodeStatus(ctx context.Context, cb func(*v1.Node)) {
	zp.nodeNotifer = cb
}

// Capacity returns a resource list containing the capacity limits.
func (zp *ZhstProvider) capacity() v1.ResourceList {
	return v1.ResourceList{
		"cpu":    resource.MustParse(zp.config.CPU),
		"memory": resource.MustParse(zp.config.Memory),
		"pods":   resource.MustParse(zp.config.Pods),
	}
}

// NodeConditions returns a list of conditions (Ready, OutOfDisk, etc), for updates to the node status
// within Kubernetes.
func (zp *ZhstProvider) nodeConditions() []v1.NodeCondition {
	// TODO: Make this configurable
	return []v1.NodeCondition{
		{
			Type:               "Ready",
			Status:             v1.ConditionFalse,
			LastHeartbeatTime:  metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             "KubeletPending",
			Message:            "kubelet is pending.",
		},
		{
			Type:               "OutOfDisk",
			Status:             v1.ConditionFalse,
			LastHeartbeatTime:  metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             "KubeletHasSufficientDisk",
			Message:            "kubelet has sufficient disk space available",
		},
		{
			Type:               "MemoryPressure",
			Status:             v1.ConditionFalse,
			LastHeartbeatTime:  metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             "KubeletHasSufficientMemory",
			Message:            "kubelet has sufficient memory available",
		},
		{
			Type:               "DiskPressure",
			Status:             v1.ConditionFalse,
			LastHeartbeatTime:  metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             "KubeletHasNoDiskPressure",
			Message:            "kubelet has no disk pressure",
		},
		{
			Type:               "NetworkUnavailable",
			Status:             v1.ConditionFalse,
			LastHeartbeatTime:  metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             "RouteCreated",
			Message:            "RouteController created a route",
		},
	}

}

// NodeAddresses returns a list of addresses for the node status
// within Kubernetes.
func (zp *ZhstProvider) nodeAddresses() []v1.NodeAddress {
	return []v1.NodeAddress{
		{
			Type:    "InternalIP",
			Address: zp.internalIP,
		},
	}
}

// NodeDaemonEndpoints returns NodeDaemonEndpoints for the node status
// within Kubernetes.
func (zp *ZhstProvider) nodeDaemonEndpoints() v1.NodeDaemonEndpoints {
	return v1.NodeDaemonEndpoints{
		KubeletEndpoint: v1.DaemonEndpoint{
			Port: zp.daemonEndpointPort,
		},
	}
}

func defaultPodStatus(pod *v1.Pod) {
	now := metav1.NewTime(time.Now())
	pod.Status = v1.PodStatus{
		Phase:     v1.PodRunning,
		HostIP:    "1.2.3.4",
		PodIP:     "5.6.7.8",
		StartTime: &now,
		Conditions: []v1.PodCondition{
			{
				Type:   v1.PodInitialized,
				Status: v1.ConditionTrue,
			},
			{
				Type:   v1.PodReady,
				Status: v1.ConditionTrue,
			},
			{
				Type:   v1.PodScheduled,
				Status: v1.ConditionTrue,
			},
		},
	}

	for _, container := range pod.Spec.Containers {
		pod.Status.ContainerStatuses = append(pod.Status.ContainerStatuses, v1.ContainerStatus{
			Name:         container.Name,
			Image:        container.Image,
			Ready:        true,
			RestartCount: 0,
			State: v1.ContainerState{
				Running: &v1.ContainerStateRunning{
					StartedAt: now,
				},
			},
		})
	}
}

func prepareDeletePod(pod *v1.Pod) {
	now := metav1.Now()
	pod.Status.Phase = v1.PodSucceeded
	pod.Status.Reason = "ZhstProviderPodDeleted"
	for idx := range pod.Status.ContainerStatuses {
		pod.Status.ContainerStatuses[idx].Ready = false
		pod.Status.ContainerStatuses[idx].State = v1.ContainerState{
			Terminated: &v1.ContainerStateTerminated{
				Message:    "Zhst provider terminated container upon deletion",
				FinishedAt: now,
				Reason:     "ZhstProviderPodContainerDeleted",
				StartedAt:  pod.Status.ContainerStatuses[idx].State.Running.StartedAt,
			},
		}
	}
}

func isNeedIgnore(pod *v1.Pod) bool {
	isIgnore := false
	//TODO:filter namespace or deploy/daemonset
	//TODO: job filter
	if pod.Namespace == "kube-system" || pod.Namespace == "default" {
		log.L.Infof("Ignore the %s namespace pod", pod.Namespace)
		isIgnore = true
	}
	return isIgnore
}

func setNodeReady(n *v1.Node) {
	n.Status.Phase = v1.NodeRunning
	for i, c := range n.Status.Conditions {
		if c.Type != "Ready" {
			continue
		}

		c.Message = "Kubelet is ready"
		c.Reason = "KubeletReady"
		c.Status = v1.ConditionTrue
		c.LastHeartbeatTime = metav1.Now()
		c.LastTransitionTime = metav1.Now()
		n.Status.Conditions[i] = c
		return
	}
}
