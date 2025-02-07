package cmd

import (
	"context"
	"os"
	"reflect"
	"time"

	"github.com/gccloudone-aurora/aurora-controller/pkg/controllers/namespaces"
	"github.com/gccloudone-aurora/aurora-controller/pkg/controllers/serviceaccounts"
	"github.com/gccloudone-aurora/aurora-controller/pkg/signals"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
)

var imagePullSecretsCmd = &cobra.Command{
	Use:   "image-pull-secrets",
	Short: "Configure image pull secrets for Aurora resources",
	Long:  `Configure image pull secrets for Aurora resources`,
	Run: func(cmd *cobra.Command, args []string) {
		// Setup signals so we can shutdown cleanly
		stopCh := signals.SetupSignalHandler()

		// Create Kubernetes config
		cfg, err := clientcmd.BuildConfigFromFlags(apiserver, kubeconfig)
		if err != nil {
			klog.Fatalf("error building kubeconfig: %v", err)
		}

		kubeClient, err := kubernetes.NewForConfig(cfg)
		if err != nil {
			klog.Fatalf("Error building kubernetes clientset: %s", err.Error())
		}

		// Setup informers
		kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, time.Minute*5)

		// Namespaces informer
		namespaceInformer := kubeInformerFactory.Core().V1().Namespaces()

		// Serviceaccount informer
		serviceAccountsInformer := kubeInformerFactory.Core().V1().ServiceAccounts()
		// serviceAccountsLister := serviceAccountsInformer.Lister()

		// Secrets informer
		secretsInformer := kubeInformerFactory.Core().V1().Secrets()
		secretsLister := secretsInformer.Lister()

		// Setup controller
		controllerServiceAccounts := serviceaccounts.NewController(
			serviceAccountsInformer,
			func(serviceAccount *corev1.ServiceAccount) error {

				found := false
				for _, imagePullSecret := range serviceAccount.ImagePullSecrets {
					if imagePullSecret.Name == os.Getenv("AURORA_SECRET_NAME") {
						found = true
						break
					}
				}

				if !found {
					klog.Infof("Adding image pull secret to %s/%s", serviceAccount.Namespace, serviceAccount.Name)

					updated := serviceAccount.DeepCopy()
					updated.ImagePullSecrets = append(serviceAccount.ImagePullSecrets, corev1.LocalObjectReference{Name: os.Getenv("AURORA_SECRET_NAME")})

					if _, err := kubeClient.CoreV1().ServiceAccounts(serviceAccount.Namespace).Update(context.Background(), updated, metav1.UpdateOptions{}); err != nil {
						return err
					}
				}

				return nil
			},
		)

		// Setup controller
		controllerNamespaces := namespaces.NewController(
			namespaceInformer,
			func(namespace *corev1.Namespace) error {
				// Generate Secrets
				secrets := generateSecrets(namespace)

				for _, secret := range secrets {
					currentSecret, err := secretsLister.Secrets(secret.Namespace).Get(secret.Name)
					if errors.IsNotFound(err) {
						klog.Infof("creating secret %s/%s", secret.Namespace, secret.Name)
						currentSecret, err = kubeClient.CoreV1().Secrets(secret.Namespace).Create(context.Background(), secret, metav1.CreateOptions{})
						if err != nil {
							return err
						}
					}

					if !reflect.DeepEqual(secret.Data, currentSecret.Data) {
						klog.Infof("updating secret %s/%s", secret.Namespace, secret.Name)
						currentSecret.Data = secret.Data

						_, err = kubeClient.CoreV1().Secrets(secret.Namespace).Update(context.Background(), currentSecret, metav1.UpdateOptions{})
						if err != nil {
							return err
						}
					}
				}

				return nil
			},
		)

		serviceAccountsInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: controllerServiceAccounts.HandleObject,
			UpdateFunc: func(old, new interface{}) {
				newNP := new.(*corev1.ServiceAccount)
				oldNP := old.(*corev1.ServiceAccount)

				if newNP.ResourceVersion == oldNP.ResourceVersion {
					return
				}

				controllerServiceAccounts.HandleObject(new)
			},
		})

		secretsInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			UpdateFunc: func(old, new interface{}) {
				newNP := new.(*corev1.Secret)
				oldNP := old.(*corev1.Secret)

				if newNP.ResourceVersion == oldNP.ResourceVersion {
					return
				}

				controllerNamespaces.HandleObject(new)
			},
			DeleteFunc: controllerNamespaces.HandleObject,
		})

		// Start informers
		kubeInformerFactory.Start(stopCh)

		// Wait for caches
		klog.Info("Waiting for informer caches to sync")
		if ok := cache.WaitForCacheSync(stopCh, serviceAccountsInformer.Informer().HasSynced, secretsInformer.Informer().HasSynced); !ok {
			klog.Fatalf("failed to wait for caches to sync")
		}

		var quit = make(chan int)

		// Run the controllerServiceAccounts
		go func() {
			if err = controllerServiceAccounts.Run(2, stopCh); err != nil {
				klog.Fatalf("error running controller: %v", err)
			}

			close(quit)
		}()

		go func() {
			if err = controllerNamespaces.Run(2, stopCh); err != nil {
				klog.Fatalf("error running controller: %v", err)
			}

			close(quit)
		}()

		// Block, the go routines are running in the background.
		<-quit
	},
}

// generateSecrets generates secrets for Aurora platform.
func generateSecrets(namespace *corev1.Namespace) []*corev1.Secret {
	secrets := []*corev1.Secret{}

	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "core/v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      os.Getenv("AURORA_SECRET_NAME"),
			Namespace: namespace.Name,
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			".dockerconfigjson": []byte(os.Getenv("AURORA_SECRET_DOCKERCONFIGJSON")),
		},
	}

	secrets = append(secrets, secret)

	return secrets
}

func init() {
	rootCmd.AddCommand(imagePullSecretsCmd)
}
