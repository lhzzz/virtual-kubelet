package zhst

import (
	"context"
	"io"
	"io/ioutil"
	"strings"
	"time"

	"github.com/virtual-kubelet/virtual-kubelet/node/api"
	stats "github.com/virtual-kubelet/virtual-kubelet/node/api/statsv1alpha1"
	v1 "k8s.io/api/core/v1"
)

type ZhstConfig struct { // nolint:golint
	CPU    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
	Pods   string `json:"pods,omitempty"`
}

type ZhstProvider struct { // nolint:golint
	nodeName           string
	operatingSystem    string
	internalIP         string
	daemonEndpointPort int32
	pods               map[string]*v1.Pod
	config             ZhstConfig
	startTime          time.Time
	notifier           func(*v1.Pod)
}

func NewZhstProvider(providerConfig, nodeName, operatingSystem string, internalIP string, daemonEndpointPort int32) (*ZhstProvider, error) {
	return nil, nil
}

func (p *ZhstProvider) CreatePod(ctx context.Context, pod *v1.Pod) error {
	return nil
}

func (p *ZhstProvider) UpdatePod(ctx context.Context, pod *v1.Pod) error {
	return nil
}

func (p *ZhstProvider) DeletePod(ctx context.Context, pod *v1.Pod) error {
	return nil
}

func (p *ZhstProvider) GetPod(ctx context.Context, namespace, name string) (pod *v1.Pod, err error) {
	return nil, nil
}

func (p *ZhstProvider) GetContainerLogs(ctx context.Context, namespace, podName, containerName string, opts api.ContainerLogOpts) (io.ReadCloser, error) {
	return ioutil.NopCloser(strings.NewReader("")), nil
}

func (p *ZhstProvider) RunInContainer(ctx context.Context, namespace, name, container string, cmd []string, attach api.AttachIO) error {
	return nil
}

func (p *ZhstProvider) GetPodStatus(ctx context.Context, namespace, name string) (*v1.PodStatus, error) {
	return nil, nil
}

func (p *ZhstProvider) GetPods(ctx context.Context) ([]*v1.Pod, error) {
	return nil, nil
}

func (p *ZhstProvider) ConfigureNode(ctx context.Context, n *v1.Node) { // nolint:golint

}

func (p *ZhstProvider) GetStatsSummary(ctx context.Context) (*stats.Summary, error) {
	return nil, nil
}

func (p *ZhstProvider) NotifyPods(ctx context.Context, notifier func(*v1.Pod)) {
	p.notifier = notifier
}
