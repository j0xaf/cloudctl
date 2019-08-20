package pkg

import (
	"fmt"
	"strings"

	"github.com/google/uuid"

	gardencorev1alpha1 "github.com/gardener/gardener/pkg/apis/core/v1alpha1"
	"k8s.io/apimachinery/pkg/util/intstr"

	gardenv1beta1 "github.com/gardener/gardener/pkg/apis/garden/v1beta1"
	corev1 "k8s.io/api/core/v1"

	"git.f-i-ts.de/cloud-native/cloudctl/pkg/api"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ShootCredentials get shoot credentials
func (g *Gardener) ShootCredentials(uid string) (string, error) {
	shoot, err := g.GetShoot(uid)
	if err != nil {
		return "", err
	}
	secret, err := g.k8sclient.CoreV1().Secrets(shoot.GetNamespace()).Get(shoot.Name+".kubeconfig", metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	config, ok := secret.Data["kubeconfig"]
	if !ok {
		return "", fmt.Errorf("unable to extract kubeconfig from shoot secret")
	}
	return string(config), nil
}

// DeleteShoot with uid
func (g *Gardener) DeleteShoot(uid string) (*gardenv1beta1.Shoot, error) {
	shoot, err := g.GetShoot(uid)
	if err != nil {
		return shoot, err
	}
	// 'confirmation.garden.sapcloud.io/deletion': 'true'
	shoot.ObjectMeta.Annotations["confirmation.garden.sapcloud.io/deletion"] = "true"
	shoot, err = g.client.GardenV1beta1().Shoots(shoot.GetNamespace()).Update(shoot)
	if err != nil {
		return shoot, err
	}
	err = g.client.GardenV1beta1().Shoots(shoot.GetNamespace()).Delete(shoot.Name, &metav1.DeleteOptions{})
	return shoot, err
}

// GetShoot shot with uid
func (g *Gardener) GetShoot(uid string) (*gardenv1beta1.Shoot, error) {
	shoots, err := g.client.GardenV1beta1().Shoots("").List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var shoot *gardenv1beta1.Shoot
	for _, s := range shoots.Items {
		if string(s.Status.UID) == uid {
			shoot = &s
			break
		}
	}
	if shoot == nil {
		return nil, fmt.Errorf("unable to find shoot for uid:%s", uid)
	}
	return shoot, nil
}

// ListShoots list all shoots
func (g *Gardener) ListShoots() ([]gardenv1beta1.Shoot, error) {
	shootList, err := g.client.GardenV1beta1().Shoots("").List(metav1.ListOptions{})
	if err != nil {
		fmt.Printf("Error listing shoots: %v", err)
		return nil, err
	}
	return shootList.Items, nil
}

// CreateShoot create a shoot for a project
func (g *Gardener) CreateShoot(scr *api.ShootCreateRequest) (*gardenv1beta1.Shoot, error) {
	p, err := g.CreateProject(scr.Owner)
	if err != nil {
		return nil, err
	}

	partition := scr.Zones[0]
	sb, err := g.CreateSecretBinding(p, partition)
	if err != nil {
		return nil, err
	}

	project, err := g.client.GardenV1beta1().Projects().Get(p.GetName(), metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	var createdBy string
	if project.Spec.CreatedBy != nil {
		createdBy = project.Spec.CreatedBy.Name
	}

	maxSurge := intstr.FromInt(scr.Workers[0].MaxSurge)
	maxUnavailable := intstr.FromInt(scr.Workers[0].MaxUnavailable)

	// FIXME
	nodesCIDR := gardencorev1alpha1.CIDR("10.250.0.0/16")
	podsCIDR := gardencorev1alpha1.CIDR("10.242.0.0/16")
	servicesCIDR := gardencorev1alpha1.CIDR("10.243.0.0/16")

	// FIXME helper method
	region := strings.Split(partition, "-")[0]

	name := scr.Name
	if name == "" {
		name = uuid.Must(uuid.NewRandom()).String()[:10]
	}

	networks := []string{}
	for _, nw := range scr.Networks {
		nwOfPartition, ok := networksOfPartition[partition]
		if !ok {
			continue
		}
		network, ok := nwOfPartition[nw]
		if !ok {
			continue
		}
		networks = append(networks, network)
	}

	shoot := &gardenv1beta1.Shoot{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Annotations: map[string]string{
				"garden.sapcloud.io/createdBy":     createdBy,
				"garden.sapcloud.io/purpose":       *scr.Purpose,
				"cluster.metal-pod.io/project":     project.Name,
				"cluster.metal-pod.io/description": *scr.Description,
				"cluster.metal-pod.io/name":        scr.Name,
				"cluster.metal-pod.io/tenant":      scr.Tenant,
			},
			Namespace: *project.Spec.Namespace,
		},
		Spec: gardenv1beta1.ShootSpec{
			Addons: &gardenv1beta1.Addons{
				KubernetesDashboard: &gardenv1beta1.KubernetesDashboard{
					Addon: gardenv1beta1.Addon{Enabled: false},
				},
				NginxIngress: &gardenv1beta1.NginxIngress{
					Addon: gardenv1beta1.Addon{Enabled: true},
				},
			},
			Cloud: gardenv1beta1.Cloud{
				Profile: "metal",
				Region:  region,
				SecretBindingRef: corev1.LocalObjectReference{
					Name: sb.Name,
				},
				Metal: &gardenv1beta1.MetalCloud{
					LoadBalancerProvider: scr.LoadBalancerProvider,
					MachineImage: &gardenv1beta1.MachineImage{
						Name:    scr.MachineImage.Name,
						Version: scr.MachineImage.Version,
					},
					FirewallImage: scr.FirewallImage,
					FirewallSize:  scr.FirewallSize,
					Networks: gardenv1beta1.MetalNetworks{
						K8SNetworks: gardencorev1alpha1.K8SNetworks{
							Nodes:    &nodesCIDR,
							Pods:     &podsCIDR,
							Services: &servicesCIDR,
						},
						Additional: networks,
					},
					Workers: []gardenv1beta1.MetalWorker{
						gardenv1beta1.MetalWorker{
							Worker: gardenv1beta1.Worker{
								Name:           scr.Workers[0].Name,
								MachineType:    scr.Workers[0].MachineType,
								AutoScalerMin:  scr.Workers[0].AutoScalerMin,
								AutoScalerMax:  scr.Workers[0].AutoScalerMax,
								MaxSurge:       &maxSurge,
								MaxUnavailable: &maxUnavailable,
							},
							VolumeType: scr.Workers[0].VolumeType,
							VolumeSize: scr.Workers[0].VolumeSize,
						},
					},
					Zones: scr.Zones,
				},
			},
			Kubernetes: gardenv1beta1.Kubernetes{
				AllowPrivilegedContainers: &scr.Kubernetes.AllowPrivilegedContainers,
				Version:                   scr.Kubernetes.Version,
			},
			Maintenance: &gardenv1beta1.Maintenance{
				AutoUpdate: &gardenv1beta1.MaintenanceAutoUpdate{
					KubernetesVersion: scr.Maintenance.AutoUpdate.KubernetesVersion,
					// MachineImageVersion: &autoUpdate,
				},
				TimeWindow: &gardenv1beta1.MaintenanceTimeWindow{
					Begin: scr.Maintenance.TimeWindow.Begin,
					End:   scr.Maintenance.TimeWindow.End,
				},
			},
		},
	}

	return g.client.GardenV1beta1().Shoots(*project.Spec.Namespace).Create(shoot)
	// 	apiVersion: garden.sapcloud.io/v1beta1
	// kind: Shoot
	// metadata:
	//     annotations:
	//         garden.sapcloud.io/createdBy: heinz.schenk@f-i-ts.de
	//         garden.sapcloud.io/purpose: production # will prevent a default hibernation schedule...
	//         cluster.metal-pod.io/project: ice-deployment
	//     name: <auto-generated-by-gardener> # maximum 10 characters
	//     namespace: garden-<cluster-id>
	// spec:
	//     addons:
	//         kubernetes-dashboard:
	//             enabled: false
	//         nginx-ingress:
	//             enabled: false # would deploy one load balancer type service, which ip address we do not want to give away just like that... it's also unclear from which network it should grab an ip
	//     cloud:
	//         metal:
	//             tenant: hlb
	//             firewallImage: firewall-1
	//             firewallSize: c1-xlarge-x86
	//             loadbalancer:
	//                 enabled: true
	//                 networks:
	//                 - count: 1 # one for vpn-shoot is required from us, it is important that vpn connection gets established otherwise the cluster is not "healthy" because api server can't reach the workers
	//                   name: internet-nbg-w8101
	//                 loadBalancerProvider: metallb
	//             machineImage:
	//                 name: metal
	//                 version: ubuntu-19.04
	//             networks:
	//                 additional:
	//                 - <external-networks>
	//                 nodes: 10.250.0.0/16
	//                 pods: 10.242.0.0/16
	//                 services: 10.243.0.0/16
	//             workers:
	//             -   autoScalerMax: 1
	//                 autoScalerMin: 1
	//                 machineType: c1-xlarge-x86
	//                 maxSurge: 1
	//                 maxUnavailable: 0
	//                 name: worker-x1a35
	//                 volumeSize: 50Gi # not interesting for us as it is bound to the machine type
	//                 volumeType: storage_1 # not interesting for us as it is bound to the machine type
	//             zones:
	//             - nbg-w8101
	//             profile: metal
	//             region: nbg
	//         secretBindingRef:
	//             name: garden-<cluster-id>
	//             seed: garden-<cluster-id>
	//     kubernetes:
	//         allowPrivilegedContainers: true
	//         version: 1.14.3
	//     maintenance:
	//         autoUpdate:
	//         kubernetesVersion: true
	//         timeWindow:
	//         begin: 230000+0000
	//         end: 000000+0000

}
