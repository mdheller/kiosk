package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	configv1alpha1 "github.com/kiosk-sh/kiosk/pkg/apis/config/v1alpha1"
	"github.com/kiosk-sh/kiosk/pkg/util/convert"
	testingutil "github.com/kiosk-sh/kiosk/pkg/util/testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type templateInstanceContollerTest struct {
	template         *configv1alpha1.Template
	templateInstance *configv1alpha1.TemplateInstance
	helmOutput       []runtime.Object
	helmError        error

	isFailed        bool
	expectedObjects []expectedStatusObject
}

type expectedStatusObject struct {
	GVK       schema.GroupVersionKind
	Name      string
	Namespace string
}

func setType(obj runtime.Object) runtime.Object {
	scheme := testingutil.NewScheme()
	gvk, err := apiutil.GVKForObject(obj, scheme)
	if err != nil {
		panic(err)
	}

	// Set the type correctly because we are to lazy to set it in the test
	accessor, err := meta.TypeAccessor(obj)
	if err != nil {
		panic(err)
	}
	accessor.SetAPIVersion(gvk.GroupVersion().String())
	accessor.SetKind(gvk.Kind)

	return obj
}

func mustConvert(obj runtime.Object) []byte {
	setType(obj)
	o, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}

	return o
}

func TestTemplateInstanceController(t *testing.T) {
	testTemplateInstance := &configv1alpha1.TemplateInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
		Spec: configv1alpha1.TemplateInstanceSpec{
			Template: "test",
		},
	}

	scheme := testingutil.NewScheme()
	tests := map[string]*templateInstanceContollerTest{
		"Simple pod": &templateInstanceContollerTest{
			templateInstance: testTemplateInstance.DeepCopy(),
			template: &configv1alpha1.Template{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Resources: configv1alpha1.TemplateResources{
					Manifests: []runtime.RawExtension{
						runtime.RawExtension{
							Raw: []byte(mustConvert(&corev1.Pod{
								ObjectMeta: metav1.ObjectMeta{
									Name: "test",
								},
								Spec: corev1.PodSpec{},
							})),
						},
					},
				},
			},
			expectedObjects: []expectedStatusObject{
				expectedStatusObject{
					GVK:       corev1.SchemeGroupVersion.WithKind("Pod"),
					Name:      "test",
					Namespace: testTemplateInstance.Namespace,
				},
			},
		},
		"Simple helm": &templateInstanceContollerTest{
			templateInstance: testTemplateInstance.DeepCopy(),
			template: &configv1alpha1.Template{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Resources: configv1alpha1.TemplateResources{
					Helm: &configv1alpha1.HelmConfiguration{
						ReleaseName: "test",
					},
				},
			},
			helmOutput: []runtime.Object{
				setType(&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
					Spec: corev1.PodSpec{},
				}),
			},
			expectedObjects: []expectedStatusObject{
				expectedStatusObject{
					GVK:       corev1.SchemeGroupVersion.WithKind("Pod"),
					Name:      "test",
					Namespace: testTemplateInstance.Namespace,
				},
			},
		},
		"Failed": &templateInstanceContollerTest{
			isFailed:         true,
			templateInstance: testTemplateInstance.DeepCopy(),
			template: &configv1alpha1.Template{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Resources: configv1alpha1.TemplateResources{
					Helm: &configv1alpha1.HelmConfiguration{},
				},
			},
			helmError: fmt.Errorf("TestError"),
		},
	}

	for testName, test := range tests {
		fakeClient := testingutil.NewFakeClient(scheme, test.template, test.templateInstance)
		fakeHelmRunner := &fakeHelmRunner{
			out: test.helmOutput,
			err: test.helmError,
		}

		controller := TemplateInstanceReconciler{
			Client: fakeClient,
			helm:   fakeHelmRunner,
			Log:    zap.New(func(o *zap.Options) {}),
			Scheme: scheme,
		}

		_, reconcileError := controller.Reconcile(reconcile.Request{NamespacedName: types.NamespacedName{Name: test.templateInstance.Name, Namespace: test.templateInstance.Namespace}})
		if reconcileError != nil {
			t.Fatal(reconcileError)
		}

		// Check if the status is equal
		err := fakeClient.Get(context.TODO(), types.NamespacedName{Name: test.templateInstance.Name}, test.templateInstance)
		if err != nil {
			t.Fatal(err)
		}
		if test.isFailed == false && test.templateInstance.Status.Status != configv1alpha1.TemplateInstanceDeploymentStatusDeployed {
			t.Fatalf("Test %s: unexpected template instance status: %s", testName, test.templateInstance.Status.Status)
		}
		if test.isFailed == true && test.templateInstance.Status.Status != configv1alpha1.TemplateInstanceDeploymentStatusFailed {
			t.Fatalf("Test %s: expected failed status, but got status %s and error %v", testName, test.templateInstance.Status.Status, reconcileError)
		}

		// Check if the runtime objects exist
		for _, obj := range test.expectedObjects {
			o, err := scheme.New(obj.GVK)
			if err != nil {
				t.Fatal(err)
			}

			err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: obj.Name, Namespace: obj.Namespace}, o)
			if err != nil {
				t.Fatalf("Test %s: expected no error retrieving %s/%s, but got %v", testName, obj.Namespace, obj.Name, err)
			}
		}
	}
}

type fakeHelmRunner struct {
	out []runtime.Object
	err error
}

func (f *fakeHelmRunner) Template(client client.Client, name, namespace string, config *configv1alpha1.HelmConfiguration) ([]*unstructured.Unstructured, error) {
	if f.err != nil {
		return nil, f.err
	}

	result := []*unstructured.Unstructured{}
	err := convert.ObjectToObject(f.out, &result)
	if err != nil {
		return nil, err
	}

	return result, nil
}
