package requestcontrol

import (
	"context"
	"time"

	errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/log"
	logutil "sigs.k8s.io/gateway-api-inference-extension/pkg/epp/util/logging"

	"k8s.io/client-go/kubernetes"
)

func InferencePoolReady(ctx context.Context, ns, labelSelector string) bool {
	logger := log.FromContext(ctx)

	c, err := k8sClient()
	if err != nil {
		logger.V(logutil.VERBOSE).Error(err, "Error extracting kubeconfig")
		return false
	}

	dc := c.AppsV1().Deployments(ns)
	deployment, err := dc.List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})

	if err != nil {
		if errors.IsNotFound(err) {
			logger.V(logutil.VERBOSE).Error(err, "Deployment for candidate pods for serving the request does not exist.")
			return false
		} else {
			logger.V(logutil.VERBOSE).Error(err, "Error getting deployment for candidate pods for serving the request")
			return false
		}
	} else {

		if len(deployment.Items) == 0 {
			logger.V(logutil.VERBOSE).Error(err, "No deployment found for candidate pods for serving the request ")
			return false
		} else {
			// Check if vLLM deployment is Ready
			count := 0
			oneReplicas := int32(1)

			for {
				deploy, err := dc.Get(ctx, deployment.Items[0].Name, metav1.GetOptions{})
				if err != nil {
					panic(err)
				}

				if deploy != nil && *deploy.Spec.Replicas >= oneReplicas && *deploy.Spec.Replicas == deploy.Status.ReadyReplicas {
					logger.V(logutil.VERBOSE).Info("Deployment for candidate pods for serving the request is READY")
					return true
				} else {
					logger.V(logutil.VERBOSE).Info("Deployment for candidate pods for serving the request is NOT READY")
				}

				time.Sleep(1 * time.Second)
				count++
				if count > 30 {
					return false
				}
			}
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
