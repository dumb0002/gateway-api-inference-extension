package requestcontrol

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/log"
	logutil "sigs.k8s.io/gateway-api-inference-extension/pkg/epp/util/logging"

	"k8s.io/client-go/kubernetes"
)

func CreateVllmEngine(ctx context.Context) bool {
	logger := log.FromContext(ctx)
	dpName := "vllm-inference-server"
	ns := "default"

	c, err := k8sClient()
	if err != nil {
		logger.V(logutil.VERBOSE).Error(err, "Error extracting kubeconfig")
		return false
	}

	dc := c.AppsV1().Deployments(ns)

	// Check if vLLM deployment already exists
	deployOld, err := dc.Get(context.TODO(), dpName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			logger.V(logutil.VERBOSE).Error(err, "Deployment '%s' in namespace '%s' does not exist.\n", dpName, ns)

			// Create a new vLLM deployment if does not exist
			dp, err := generateDeployment(dpName, ns, 1)
			if err != nil {
				logger.V(logutil.VERBOSE).Error(err, "Error generating the deployment spec")
				return false
			}

			logger.V(logutil.VERBOSE).Info("deployment", "deployment", dpName, "status", "creating")
			result, err := dc.Create(context.Background(), dp, metav1.CreateOptions{})
			if err != nil {
				logger.V(logutil.VERBOSE).Error(err, "Error creating the deployment in the cluster")
				return false
			}
			logger.V(logutil.VERBOSE).Info("deployment", "deployment", result.GetObjectMeta().GetName(), "status", "created")

		} else {
			logger.V(logutil.VERBOSE).Error(err, "Error getting deployment '%s' in namespace '%s'", dpName, ns)
			return false
		}
	} else {

		if deployOld != nil {
			// if vLLM deployment exists but has 0 replicas then set the number of replicas to 1
			if *deployOld.Spec.Replicas == 0 {
				oneReplicas := int32(1)
				deployOld.Spec.Replicas = &oneReplicas

				_, err = dc.Update(context.TODO(), deployOld, metav1.UpdateOptions{})
				if err != nil {
					logger.V(logutil.VERBOSE).Error(err, "Error scaling up the deployment replicas to one")
					return false
				}
				logger.V(logutil.VERBOSE).Info(fmt.Sprintf("Deployment '%s' in namespace '%s' scaled up to one replica.\n", dpName, ns))
			} else {
				// if vLLM deployment exists and has no replicas then do not create a new vLLM deployment
				logger.V(logutil.VERBOSE).Info(fmt.Sprintf("Deployment %s already exists and it has no zero replicas", dpName))
			}
		}
	}

	// Check if vLLM deployment is Ready
	count := 0
	for {
		deploy, err := dc.Get(context.Background(), dpName, metav1.GetOptions{})
		if err != nil {
			panic(err)
		}

		if deploy != nil && deploy.Spec.Replicas != nil && *deploy.Spec.Replicas == deploy.Status.ReadyReplicas {
			logger.V(logutil.VERBOSE).Info("Deployment is READY")
			return true
		} else {
			logger.V(logutil.VERBOSE).Info("Deployment is NOT READY")
		}

		time.Sleep(1 * time.Second)
		count++
		if count > 30 {
			return false
		}
	}
}

func k8sClient() (*kubernetes.Clientset, error) {
	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(kubeConfig)
	return clientset, err
}

func generateDeployment(deploymentName, namespace string, replicas int32) (*appsv1.Deployment, error) {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: namespace,
			Labels: map[string]string{
				"app": "food-review-inference-pool",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "food-review-inference-pool",
				},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "food-review-inference-pool",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:            "vllm",
							Image:           fmt.Sprintf("ghcr.io/llm-d/llm-d-inference-sim:latest"),
							ImagePullPolicy: v1.PullIfNotPresent,
							Args: []string{
								"--port=8000",
								"--model=food-review",
								"--enable-kvcache=false",
								"--kv-cache-size=1024",
								"--block-size=16",
								"--zmq-endpoint=tcp://food-review-endpoint-picker.default.svc.cluster.local:5557",
								"--event-batch-size=16",
								"--tokenizers-cache-dir=/tokenizer-cache",
							},
							Ports: []v1.ContainerPort{
								{
									Name:          "http",
									Protocol:      v1.ProtocolTCP,
									ContainerPort: 8000,
								},
							},
							Env: []v1.EnvVar{
								{
									Name:  "PORT",
									Value: "8000",
								},
								{
									Name:  "PYTHONHASHSEED",
									Value: "42",
								},
							},
							VolumeMounts: []v1.VolumeMount{
								{
									MountPath: "/tokenizer-cache",
									Name:      "tokenizer-cache",
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name:         "tokenizer-cache",
							VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}},
						},
					},
				},
			},
		},
	}

	return deployment, nil
}
