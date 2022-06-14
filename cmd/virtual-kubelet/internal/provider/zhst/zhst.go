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
	"github.com/virtual-kubelet/virtual-kubelet/log"
	"github.com/virtual-kubelet/virtual-kubelet/node/api"
	stats "github.com/virtual-kubelet/virtual-kubelet/node/api/statsv1alpha1"
	"google.golang.org/grpc"
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
}

func (z *ZhstProvider) getEdgeletClient() (pb.EdgeletClient, error) {
	if z.edgeConnect != nil {
		return pb.NewEdgeletClient(z.edgeConnect), nil
	}
	if len(z.config.EdgeAddress) == 0 {
		return nil, errors.New("edge address is empty")
	}
	edgeconn, err := grpc.Dial(z.config.EdgeAddress, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	z.edgeConnect = edgeconn
	return pb.NewEdgeletClient(edgeconn), nil
}

func NewZhstProvider(providerConfig, nodeName, operatingSystem string, internalIP string, daemonEndpointPort int32) (*ZhstProvider, error) {
	config, err := loadConfig(providerConfig, nodeName)
	if err != nil {
		return nil, err
	}
	return &ZhstProvider{
		config:             config,
		nodeName:           nodeName,
		operatingSystem:    operatingSystem,
		internalIP:         internalIP,
		daemonEndpointPort: daemonEndpointPort,
		startTime:          time.Now(),
	}, nil
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

func (p *ZhstProvider) CreatePod(ctx context.Context, pod *v1.Pod) error {
	client, err := p.getEdgeletClient()
	if err != nil {
		return err
	}
	resp, err := client.CreatePod(ctx, &pb.CreatePodRequest{
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

func (p *ZhstProvider) UpdatePod(ctx context.Context, pod *v1.Pod) error {
	client, err := p.getEdgeletClient()
	if err != nil {
		return err
	}
	resp, err := client.UpdatePod(ctx, &pb.UpdatePodRequest{
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

func (p *ZhstProvider) DeletePod(ctx context.Context, pod *v1.Pod) error {
	client, err := p.getEdgeletClient()
	if err != nil {
		return err
	}
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

func (p *ZhstProvider) GetPod(ctx context.Context, namespace, name string) (pod *v1.Pod, err error) {
	client, err := p.getEdgeletClient()
	if err != nil {
		return nil, err
	}
	resp, err := client.GetPod(ctx, &pb.GetPodRequest{
		Namespace: namespace,
		Name:      name,
	})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf(resp.Error.Msg)
	}
	return resp.Pod, nil
}

func (p *ZhstProvider) GetContainerLogs(ctx context.Context, namespace, podName, containerName string, opts api.ContainerLogOpts) (io.ReadCloser, error) {
	return ioutil.NopCloser(strings.NewReader("")), nil
}

func (p *ZhstProvider) RunInContainer(ctx context.Context, namespace, name, container string, cmd []string, attach api.AttachIO) error {
	return nil
}

func (p *ZhstProvider) GetPodStatus(ctx context.Context, namespace, name string) (*v1.PodStatus, error) {
	client, err := p.getEdgeletClient()
	if err != nil {
		return nil, err
	}
	resp, err := client.GetPodStatus(ctx, &pb.GetPodStatusRequest{
		Namespace: namespace,
		Name:      name,
	})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf(resp.Error.Msg)
	}
	return resp.PodStatus, nil
}

func (p *ZhstProvider) GetPods(ctx context.Context) ([]*v1.Pod, error) {
	client, err := p.getEdgeletClient()
	if err != nil {
		return nil, err
	}
	resp, err := client.GetPods(ctx, &pb.GetPodsRequest{})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf(resp.Error.Msg)
	}
	return resp.Pods, nil
}

func (p *ZhstProvider) ConfigureNode(ctx context.Context, n *v1.Node) { // nolint:golint
	n.Status.Capacity = p.capacity()
	n.Status.Allocatable = p.capacity()
	n.Status.Conditions = p.nodeConditions()
	n.Status.Addresses = p.nodeAddresses()
	n.Status.DaemonEndpoints = p.nodeDaemonEndpoints()
	os := p.operatingSystem
	if os == "" {
		os = "linux"
	}
	n.Status.NodeInfo.OperatingSystem = os
	n.Status.NodeInfo.Architecture = "amd64"
	n.ObjectMeta.Labels["alpha.service-controller.kubernetes.io/exclude-balancer"] = "true"
	n.ObjectMeta.Labels["node.kubernetes.io/exclude-from-external-load-balancers"] = "true"
}

func (p *ZhstProvider) GetStatsSummary(ctx context.Context) (*stats.Summary, error) {
	return nil, nil
}

func (p *ZhstProvider) NotifyPods(ctx context.Context, notifier func(*v1.Pod)) {
	p.notifier = notifier
}

// Capacity returns a resource list containing the capacity limits.
func (p *ZhstProvider) capacity() v1.ResourceList {
	return v1.ResourceList{
		"cpu":    resource.MustParse(p.config.CPU),
		"memory": resource.MustParse(p.config.Memory),
		"pods":   resource.MustParse(p.config.Pods),
	}
}

// NodeConditions returns a list of conditions (Ready, OutOfDisk, etc), for updates to the node status
// within Kubernetes.
func (p *ZhstProvider) nodeConditions() []v1.NodeCondition {
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
func (p *ZhstProvider) nodeAddresses() []v1.NodeAddress {
	return []v1.NodeAddress{
		{
			Type:    "InternalIP",
			Address: p.internalIP,
		},
	}
}

// NodeDaemonEndpoints returns NodeDaemonEndpoints for the node status
// within Kubernetes.
func (p *ZhstProvider) nodeDaemonEndpoints() v1.NodeDaemonEndpoints {
	return v1.NodeDaemonEndpoints{
		KubeletEndpoint: v1.DaemonEndpoint{
			Port: p.daemonEndpointPort,
		},
	}
}
