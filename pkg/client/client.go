package client

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// UnstructuredClient wraps a controller-runtime client to use unstructured
// resources for Get/List by default, with exemptions for specific GVKs.
// This ensures a unified caching strategy where all non-exempted resources
// go through the unstructured cache path.
type Client struct {
	inner    client.Client
	uncached client.Reader
}

// Option configures the UnstructuredClient.
type Option func(*Client)

// WithUncachedReader sets a reader used when the cache-backed client rejects a
// Get/List because the object's namespace is not in the informer cache.
func WithUncachedReader(r client.Reader) Option {
	return func(c *Client) {
		c.uncached = r
	}
}

// New creates a new Client that wraps the given client.
// By default, all Get/List operations use unstructured resources unless the GVK
// is exempted via WithTypedGVKs.
func New(inner client.Client, opts ...Option) *Client {
	c := &Client{
		inner: inner,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Get retrieves an object for the given key.
// To ensure consistent cache access:
//   - If caller passes typed object: use unstructured for cache and return typed object
//   - If caller passes unstructured: use unstructured for cache and return unstructured object
func (c *Client) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	log := logf.FromContext(ctx)

	gvk, err := apiutil.GVKForObject(obj, c.Scheme())
	if err != nil {
		return fmt.Errorf("failed to get GVK: %w", err)
	}

	_, isUnstructured := obj.(*unstructured.Unstructured)

	switch {
	case !isUnstructured:
		// Caller passed typed → use unstructured for cache
		log.V(1).Info("Client.Get: typed input, non-exempted GVK - using unstructured cache with conversion",
			"gvk", gvk, "key", key, "inputType", "typed", "cacheType", "unstructured", "converted", true)
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(gvk)
		if err := c.get(ctx, key, u, gvk, opts...); err != nil {
			return err
		}
		// Convert unstructured result back to typed
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, obj); err != nil {
			return fmt.Errorf("failed to convert unstructured to typed: %w", err)
		}
		obj.GetObjectKind().SetGroupVersionKind(gvk)
		return nil

	default:
		// No conversion needed - input type matches cache type
		log.V(1).Info("Client.Get: no conversion needed - input type matches cache type",
			"gvk", gvk, "key", key, "isUnstructured", isUnstructured, "converted", false)
		return c.get(ctx, key, obj, gvk, opts...)
	}
}

// List retrieves a list of objects.
// To ensure consistent cache access:
//   - If caller passes typed list: use unstructured for cache and return typed list
//   - If caller passes unstructured list: use unstructured for cache and return unstructured list
func (c *Client) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	log := logf.FromContext(ctx)

	gvk, err := apiutil.GVKForObject(list, c.Scheme())
	if err != nil {
		return fmt.Errorf("failed to get GVK for list: %w", err)
	}

	_, isUnstructuredList := list.(*unstructured.UnstructuredList)

	switch {
	case !isUnstructuredList:
		// Caller passed typed list → use unstructured for cache
		log.V(1).Info("Client.List: typed input, non-exempted GVK - using unstructured cache with conversion",
			"gvk", gvk, "inputType", "typed", "cacheType", "unstructured", "converted", true)
		ul := &unstructured.UnstructuredList{}
		ul.SetGroupVersionKind(gvk)
		if err := c.list(ctx, ul, gvk, opts...); err != nil {
			return err
		}
		// Convert unstructured result back to typed
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(ul.UnstructuredContent(), list); err != nil {
			return fmt.Errorf("failed to convert unstructured to typed: %w", err)
		}
		list.GetObjectKind().SetGroupVersionKind(gvk)
		return nil

	default:
		// No conversion needed - input type matches cache type
		log.V(1).Info("Client.List: no conversion needed - input type matches cache type",
			"gvk", gvk, "isUnstructuredList", isUnstructuredList, "converted", false)
		return c.list(ctx, list, gvk, opts...)
	}
}

func (c *Client) get(ctx context.Context, key client.ObjectKey, obj client.Object, gvk schema.GroupVersionKind, opts ...client.GetOption) error {
	err := c.inner.Get(ctx, key, obj, opts...)
	if err == nil || c.uncached == nil || !isUnknownNamespaceForCache(err) {
		return err
	}

	logf.FromContext(ctx).Info("cache does not include namespace, retrying with uncached client",
		"namespace", key.Namespace,
		"name", key.Name,
		"gvk", gvk,
	)
	return c.uncached.Get(ctx, key, obj, opts...)
}

func (c *Client) list(ctx context.Context, list client.ObjectList, gvk schema.GroupVersionKind, opts ...client.ListOption) error {
	err := c.inner.List(ctx, list, opts...)
	if err == nil || c.uncached == nil || !isUnknownNamespaceForCache(err) {
		return err
	}

	listOpts := client.ListOptions{}
	listOpts.ApplyOptions(opts)
	logf.FromContext(ctx).Info("cache does not include namespace, retrying with uncached client",
		"namespace", listOpts.Namespace,
		"gvk", gvk,
	)
	return c.uncached.List(ctx, list, opts...)
}

func isUnknownNamespaceForCache(err error) bool {
	return err != nil && strings.Contains(err.Error(), "unknown namespace for the cache")
}

// Create saves the object in the Kubernetes cluster.
func (c *Client) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	return c.inner.Create(ctx, obj, opts...)
}

// Delete deletes the given object from the Kubernetes cluster.
func (c *Client) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	return c.inner.Delete(ctx, obj, opts...)
}

// Update updates the given object in the Kubernetes cluster.
func (c *Client) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	return c.inner.Update(ctx, obj, opts...)
}

// Patch patches the given object in the Kubernetes cluster.
func (c *Client) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	return c.inner.Patch(ctx, obj, patch, opts...)
}

// Apply applies the given object to the Kubernetes cluster.
func (c *Client) Apply(ctx context.Context, obj runtime.ApplyConfiguration, opts ...client.ApplyOption) error {
	return c.inner.Apply(ctx, obj, opts...)
}

// DeleteAllOf deletes all objects of the given type matching the given options.
func (c *Client) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	return c.inner.DeleteAllOf(ctx, obj, opts...)
}

// Status returns a client for the status subresource.
func (c *Client) Status() client.SubResourceWriter {
	return c.inner.Status()
}

// SubResource returns a client for the named subresource.
func (c *Client) SubResource(subResource string) client.SubResourceClient {
	return c.inner.SubResource(subResource)
}

// Scheme returns the scheme this client is using.
func (c *Client) Scheme() *runtime.Scheme {
	return c.inner.Scheme()
}

// RESTMapper returns the REST mapper this client is using.
func (c *Client) RESTMapper() meta.RESTMapper {
	return c.inner.RESTMapper()
}

// GroupVersionKindFor returns the GroupVersionKind for the given object.
func (c *Client) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	return c.inner.GroupVersionKindFor(obj)
}

// IsObjectNamespaced returns true if the GroupVersionKind of the object is namespaced.
func (c *Client) IsObjectNamespaced(obj runtime.Object) (bool, error) {
	return c.inner.IsObjectNamespaced(obj)
}

// Ensure Client implements client.Client at compile time.
var _ client.Client = (*Client)(nil)
