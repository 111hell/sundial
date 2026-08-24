package sundial_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/sundayfun/sundial"
	yamlcodec "github.com/sundayfun/sundial/codec/yaml"
	providertesting "github.com/sundayfun/sundial/provider/testing"
)

type prefixedJSONCodec struct {
	prefix []byte
}

func (c prefixedJSONCodec) Encode(value any) ([]byte, error) {
	data, err := json.Marshal(value)
	return append(append([]byte(nil), c.prefix...), data...), err
}

func (c prefixedJSONCodec) Decode(data []byte, value any) error {
	if !bytes.HasPrefix(data, c.prefix) {
		return errors.New("missing custom prefix")
	}
	return json.Unmarshal(bytes.TrimPrefix(data, c.prefix), value)
}

func TestNewLoadsConfigurationIntoMemory(t *testing.T) {
	t.Parallel()

	provider := providertesting.New([]byte(`{"server":{"port":8080},"ratio":1.5,"enabled":true}`))
	config, err := sundial.New(context.Background(), sundial.Options{
		Provider: provider,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if got := config.Int("server.port"); got != 8080 {
		t.Fatalf("Int(server.port) = %d, want 8080", got)
	}
	if got := config.Int("ratio"); got != 0 {
		t.Fatalf("Int(ratio) = %d, want 0 for a fractional value", got)
	}
	if !config.Bool("enabled") {
		t.Fatal("Bool(enabled) = false, want true")
	}
	if got := provider.LoadCount(); got != 1 {
		t.Fatalf("LoadCount() = %d, want 1", got)
	}

	for range 10 {
		_ = config.Int("server.port")
	}
	if got := provider.LoadCount(); got != 1 {
		t.Fatalf("memory reads called Provider.Load: count = %d", got)
	}

	var server struct {
		Port int `json:"port"`
	}
	if err := config.Unmarshal("server", &server); err != nil {
		t.Fatalf("Unmarshal(server) error = %v", err)
	}
	if server.Port != 8080 {
		t.Fatalf("Unmarshal(server).Port = %d, want 8080", server.Port)
	}
}

func TestMissingConfigurationStartsEmpty(t *testing.T) {
	t.Parallel()

	config, err := sundial.New(context.Background(), sundial.Options{
		Provider: providertesting.New(nil),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if config.Exists("server") {
		t.Fatal("Exists(server) = true, want false")
	}
}

func TestNewRequiresProvider(t *testing.T) {
	t.Parallel()

	_, err := sundial.New(context.Background(), sundial.Options{})
	if !errors.Is(err, sundial.ErrProviderRequired) {
		t.Fatalf("New() error = %v, want ErrProviderRequired", err)
	}
}

func TestCustomCodec(t *testing.T) {
	t.Parallel()

	const prefix = "custom:"
	codec := prefixedJSONCodec{prefix: []byte(prefix)}
	provider := providertesting.New([]byte(`custom:{"enabled":true}`))
	config, err := sundial.New(context.Background(), sundial.Options{
		Provider: provider,
		Codec:    codec,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if !config.Bool("enabled") {
		t.Fatal("Bool(enabled) = false, want true")
	}

	if err := config.Set(context.Background(), "enabled", false); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if !bytes.HasPrefix(provider.Data(), []byte(prefix)) {
		t.Fatalf("saved data = %q, want custom prefix", provider.Data())
	}
}

func TestYAMLCodec(t *testing.T) {
	t.Parallel()

	provider := providertesting.New([]byte("server:\n  port: 8080\n"))
	config, err := sundial.New(context.Background(), sundial.Options{
		Provider: provider,
		Codec:    yamlcodec.New(),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if got := config.Int("server.port"); got != 8080 {
		t.Fatalf("Int(server.port) = %d, want 8080", got)
	}

	if err := config.Set(context.Background(), "server.port", 9090); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if !bytes.Contains(provider.Data(), []byte("port: 9090")) {
		t.Fatalf("saved data = %q, want YAML port 9090", provider.Data())
	}
}

func TestAddSetDeletePersistCompleteDocument(t *testing.T) {
	t.Parallel()

	provider := providertesting.New(nil)
	config, err := sundial.New(context.Background(), sundial.Options{
		Provider: provider,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = config.Add(context.Background(), "server", map[string]any{
		"host": "127.0.0.1",
		"port": 8080,
	})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if err := config.Add(context.Background(), "server", map[string]any{}); !errors.Is(err, sundial.ErrAlreadyExists) {
		t.Fatalf("second Add() error = %v, want ErrAlreadyExists", err)
	}
	if err := config.Set(context.Background(), "server.port", 9090); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if err := config.Delete(context.Background(), "server.host"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if got := config.Int("server.port"); got != 9090 {
		t.Fatalf("Int(server.port) = %d, want 9090", got)
	}
	if config.Exists("server.host") {
		t.Fatal("Exists(server.host) = true, want false")
	}
	if got := provider.SaveCount(); got != 3 {
		t.Fatalf("SaveCount() = %d, want 3", got)
	}

	reloaded, err := sundial.New(context.Background(), sundial.Options{
		Provider: provider,
	})
	if err != nil {
		t.Fatalf("reload New() error = %v", err)
	}
	if got := reloaded.Int("server.port"); got != 9090 {
		t.Fatalf("reloaded Int(server.port) = %d, want 9090", got)
	}
}

func TestSaveFailureKeepsPreviousMemory(t *testing.T) {
	t.Parallel()

	provider := providertesting.New([]byte(`{"server":{"port":8080}}`))
	config, err := sundial.New(context.Background(), sundial.Options{
		Provider: provider,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	provider.SetSaveError(errors.New("save failed"))
	if err := config.Set(context.Background(), "server.port", 9090); err == nil {
		t.Fatal("Set() error = nil, want failure")
	}
	if got := config.Int("server.port"); got != 8080 {
		t.Fatalf("Int(server.port) = %d after failed save, want 8080", got)
	}
}

func TestSetRejectsPathConflict(t *testing.T) {
	t.Parallel()

	config, err := sundial.New(context.Background(), sundial.Options{
		Provider: providertesting.New([]byte(`{"server":"disabled"}`)),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := config.Set(context.Background(), "server.port", 8080); !errors.Is(err, sundial.ErrPathConflict) {
		t.Fatalf("Set() error = %v, want ErrPathConflict", err)
	}
}

func TestReloadFailureKeepsPreviousMemory(t *testing.T) {
	t.Parallel()

	provider := providertesting.New([]byte(`{"server":{"port":8080}}`))
	config, err := sundial.New(context.Background(), sundial.Options{
		Provider: provider,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	provider.SetData([]byte(`{"server":`))
	if err := config.Reload(context.Background()); err == nil {
		t.Fatal("Reload() error = nil, want parse failure")
	}
	if got := config.Int("server.port"); got != 8080 {
		t.Fatalf("Int(server.port) = %d after failed reload, want 8080", got)
	}
}

func TestGetReturnsDetachedValues(t *testing.T) {
	t.Parallel()

	provider := providertesting.New([]byte(`{"server":{"tags":["api"]}}`))
	config, err := sundial.New(context.Background(), sundial.Options{
		Provider: provider,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	server := config.Get("server").(map[string]any)
	server["tags"].([]any)[0] = "mutated"

	tags := config.Get("server.tags").([]any)
	if got := tags[0]; got != "api" {
		t.Fatalf("server.tags[0] = %v, want api", got)
	}
}

func TestWatchPollingReloadsExternalChanges(t *testing.T) {
	t.Parallel()

	provider := providertesting.New([]byte(`{"server":{"port":8080}}`))
	config, err := sundial.New(context.Background(), sundial.Options{
		Provider:      provider,
		WatchInterval: 5 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	changed := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		done <- config.Watch(ctx, sundial.WatchOptions{
			OnChange: func() { changed <- struct{}{} },
		})
	}()

	provider.SetData([]byte(`{"server":{"port":9090}}`))
	select {
	case <-changed:
	case <-time.After(time.Second):
		t.Fatal("Watch did not report external change")
	}
	if got := config.Int("server.port"); got != 9090 {
		t.Fatalf("Int(server.port) = %d, want 9090", got)
	}

	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Watch() error = %v, want context.Canceled", err)
	}
}

func TestNativeWatchReloadsExternalChanges(t *testing.T) {
	t.Parallel()

	provider := providertesting.NewWatcher([]byte(`{"enabled":false}`))
	config, err := sundial.New(context.Background(), sundial.Options{
		Provider: provider,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	changed := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		done <- config.Watch(ctx, sundial.WatchOptions{
			OnChange: func() { changed <- struct{}{} },
		})
	}()

	provider.Change([]byte(`{"enabled":true}`))
	select {
	case <-changed:
	case <-time.After(time.Second):
		t.Fatal("native Watch did not report external change")
	}
	if !config.Bool("enabled") {
		t.Fatal("Bool(enabled) = false, want true")
	}

	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Watch() error = %v, want context.Canceled", err)
	}
}

func TestConcurrentReadsAndWrites(t *testing.T) {
	t.Parallel()

	provider := providertesting.New([]byte(`{"counter":0}`))
	config, err := sundial.New(context.Background(), sundial.Options{
		Provider: provider,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	var group sync.WaitGroup
	for i := range 20 {
		group.Add(2)
		go func(value int) {
			defer group.Done()
			if err := config.Set(context.Background(), "counter", value); err != nil {
				t.Errorf("Set() error = %v", err)
			}
		}(i)
		go func() {
			defer group.Done()
			_ = config.Int("counter")
			_ = config.Raw()
		}()
	}
	group.Wait()
}
