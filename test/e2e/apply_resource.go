package e2e

import (
	"bytes"
	"context"
	"os"
	"time"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateFromFile(ctx context.Context, k8sClient client.Client, namespace, path string) (func(), error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return func() {}, errors.Wrap(err, "failed to read file content")
	}

	content = filterCommentsAndEmptyLines(content)
	resources := bytes.Split(content, []byte("\n---"))

	decoder, err := defaultDecoder()
	if err != nil {
		return func() {}, errors.Wrap(err, "failed to initialize decoder")
	}

	objects := []client.Object{}

	for _, res := range resources {
		if len(res) == 0 {
			continue
		}

		runtimeObject, _, err := decoder.Decode(res, nil, nil)
		if err != nil {
			return func() {}, errors.Wrap(err, "failed to decode runtimeObject")
		}

		object, ok := runtimeObject.(client.Object)
		if !ok {
			return func() {}, errors.New("failed to get runtimeObject metadata")
		}

		object.SetNamespace(namespace)

		err = k8sClient.Create(ctx, object)
		if err != nil {
			return func() {}, errors.Wrap(err, "failed to apply runtimeObject")
		}

		objects = append(objects, object)
	}

	cleanup := func() {
		ctx := context.Background()
		for _, obj := range objects {
			_ = k8sClient.Delete(ctx, obj)
		}
		// Wait for all objects to be fully gone so the next test can recreate them.
		for _, obj := range objects {
			key := client.ObjectKeyFromObject(obj)
			_ = wait.PollUntilContextTimeout(ctx, 2*time.Second, 2*time.Minute, true,
				func(ctx context.Context) (bool, error) {
					err := k8sClient.Get(ctx, key, obj)
					if apierrors.IsNotFound(err) {
						return true, nil
					}
					return false, nil
				})
		}
	}

	return cleanup, nil
}

func filterCommentsAndEmptyLines(fileContent []byte) []byte {
	lines := bytes.Split(fileContent, []byte("\n"))

	newLines := make([][]byte, 0, len(lines))

	for _, l := range lines {
		if !bytes.HasPrefix(l, []byte("#")) && len(bytes.TrimSpace(l)) != 0 {
			newLines = append(newLines, l)
		}
	}

	return bytes.Join(newLines, []byte("\n"))
}

func defaultScheme() (*runtime.Scheme, error) {
	resourcesSchema := runtime.NewScheme()

	var addToSchemes = []func(*runtime.Scheme) error{
		scheme.AddToScheme,
		mmv1beta.AddToScheme,
	}

	for _, f := range addToSchemes {
		err := f(resourcesSchema)
		if err != nil {
			return nil, errors.Wrap(err, "failed to add types to schema")
		}
	}

	return resourcesSchema, nil
}

func defaultDecoder() (runtime.Decoder, error) {
	resourceScheme, err := defaultScheme()
	if err != nil {
		return nil, err
	}
	codecs := serializer.NewCodecFactory(resourceScheme)
	decoder := codecs.UniversalDeserializer()

	return decoder, nil
}
