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

type testConfig struct {
	Server  serverConfig      `json:"server"`
	Enabled bool              `json:"enabled"`
	Ratio   float64           `json:"ratio"`
	Counter int               `json:"counter"`
	Labels  map[string]string `json:"labels"`
}

type serverConfig struct {
	Host string     `json:"host"`
	Port int        `json:"port"`
	Tags []string   `json:"tags"`
	TLS  *tlsConfig `json:"tls"`
}

type tlsConfig struct {
	Enabled bool `json:"enabled"`
}

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

func TestNewLoadsTypedConfigurationIntoMemory(t *testing.T) {
	t.Parallel()

	provider := providertesting.New([]byte(`{
		"server":{"host":"127.0.0.1","port":8080},
		"ratio":1.5,
		"enabled":true
	}`))
	configStore, err := sundial.New[testConfig](
		context.Background(),
		provider,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	config, _, err := configStore.Get()
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if config.Server.Port != 8080 {
		t.Fatalf("Get().Server.Port = %d, want 8080", config.Server.Port)
	}
	if config.Ratio != 1.5 {
		t.Fatalf("Get().Ratio = %v, want 1.5", config.Ratio)
	}
	if !config.Enabled {
		t.Fatal("Get().Enabled = false, want true")
	}
	if got := provider.LoadCount(); got != 1 {
		t.Fatalf("LoadCount() = %d, want 1", got)
	}

	for range 10 {
		if _, _, getErr := configStore.Get(); getErr != nil {
			t.Fatalf("Get() error = %v", getErr)
		}
	}
	if got := provider.LoadCount(); got != 1 {
		t.Fatalf("memory reads called Provider.Load: count = %d", got)
	}
}

func TestMissingConfigurationStartsWithZeroValue(t *testing.T) {
	t.Parallel()

	configStore, err := sundial.New[testConfig](
		context.Background(),
		providertesting.New(nil),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	config, _, err := configStore.Get()
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if config.Server.Host != "" || config.Server.Port != 0 || config.Server.Tags != nil ||
		config.Server.TLS != nil || config.Enabled || config.Ratio != 0 || config.Counter != 0 ||
		config.Labels != nil {
		t.Fatalf("Get() = %#v, want zero value", config)
	}
}

func TestNewRejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()

	_, err := sundial.New[testConfig](
		context.Background(),
		providertesting.New([]byte(`{"server":`)),
	)
	if err == nil {
		t.Fatal("New() error = nil, want decode failure")
	}
}

func TestCustomCodec(t *testing.T) {
	t.Parallel()

	const prefix = "custom:"
	provider := providertesting.New([]byte(`custom:{"enabled":true}`))
	configStore, err := sundial.New[testConfig](
		context.Background(),
		provider,
		sundial.WithCodec(prefixedJSONCodec{prefix: []byte(prefix)}),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	config, version, err := configStore.Get()
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	config.Enabled = false
	if putErr := configStore.Put(context.Background(), config, version); putErr != nil {
		t.Fatalf("Put() error = %v", putErr)
	}
	if !bytes.HasPrefix(provider.Data(), []byte(prefix)) {
		t.Fatalf("saved data = %q, want custom prefix", provider.Data())
	}
}

func TestYAMLCodec(t *testing.T) {
	t.Parallel()

	provider := providertesting.New([]byte("server:\n  port: 8080\n"))
	configStore, err := sundial.New[testConfig](
		context.Background(),
		provider,
		sundial.WithCodec(yamlcodec.New()),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	config, _, err := configStore.Get()
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if config.Server.Port != 8080 {
		t.Fatalf("Get().Server.Port = %d, want 8080", config.Server.Port)
	}

	config, version, err := configStore.Get()
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	config.Server.Port = 9090
	if putErr := configStore.Put(context.Background(), config, version); putErr != nil {
		t.Fatalf("Put() error = %v", putErr)
	}
	if !bytes.Contains(provider.Data(), []byte("port: 9090")) {
		t.Fatalf("saved data = %q, want YAML port 9090", provider.Data())
	}
}

func TestPutPersistsCompleteDocument(t *testing.T) {
	t.Parallel()

	provider := providertesting.New([]byte(`{
		"server":{"host":"127.0.0.1","port":8080},
		"enabled":true
	}`))
	configStore, err := sundial.New[testConfig](
		context.Background(),
		provider,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	config, version, err := configStore.Get()
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	config.Server.Port = 9090
	if putErr := configStore.Put(context.Background(), config, version); putErr != nil {
		t.Fatalf("Put() error = %v", putErr)
	}

	if got := provider.SaveCount(); got != 1 {
		t.Fatalf("SaveCount() = %d, want 1", got)
	}
	var saved testConfig
	if decodeErr := json.Unmarshal(provider.Data(), &saved); decodeErr != nil {
		t.Fatalf("decode saved configuration: %v", decodeErr)
	}
	if saved.Server.Host != "127.0.0.1" || saved.Server.Port != 9090 || !saved.Enabled {
		t.Fatalf("saved configuration = %#v, want complete updated document", saved)
	}

	reloaded, err := sundial.New[testConfig](
		context.Background(),
		provider,
	)
	if err != nil {
		t.Fatalf("reload New() error = %v", err)
	}
	reloadedConfig, _, err := reloaded.Get()
	if err != nil {
		t.Fatalf("reload Get() error = %v", err)
	}
	if reloadedConfig.Server.Port != 9090 {
		t.Fatalf("reloaded port = %d, want 9090", reloadedConfig.Server.Port)
	}
}

func TestSaveFailureKeepsPreviousMemory(t *testing.T) {
	t.Parallel()

	provider := providertesting.New([]byte(`{"server":{"port":8080}}`))
	configStore, err := sundial.New[testConfig](
		context.Background(),
		provider,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	provider.SetSaveError(errors.New("save failed"))
	config, version, err := configStore.Get()
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	config.Server.Port = 9090
	if putErr := configStore.Put(context.Background(), config, version); putErr == nil {
		t.Fatal("Put() error = nil, want failure")
	}

	current, _, err := configStore.Get()
	if err != nil {
		t.Fatalf("Get() after failed Put error = %v", err)
	}
	if current.Server.Port != 8080 {
		t.Fatalf("port after failed Put = %d, want 8080", current.Server.Port)
	}
}

func TestReloadFailureKeepsPreviousMemory(t *testing.T) {
	t.Parallel()

	provider := providertesting.New([]byte(`{"server":{"port":8080}}`))
	configStore, err := sundial.New[testConfig](
		context.Background(),
		provider,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	provider.SetData([]byte(`{"server":`))
	if reloadErr := configStore.Reload(context.Background()); reloadErr == nil {
		t.Fatal("Reload() error = nil, want decode failure")
	}
	config, _, err := configStore.Get()
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if config.Server.Port != 8080 {
		t.Fatalf("port after failed reload = %d, want 8080", config.Server.Port)
	}
}

func TestGetReturnsDetachedConfiguration(t *testing.T) {
	t.Parallel()

	provider := providertesting.New([]byte(`{
		"server":{"tags":["api"],"tls":{"enabled":true}},
		"labels":{"region":"east"}
	}`))
	configStore, err := sundial.New[testConfig](
		context.Background(),
		provider,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	config, _, err := configStore.Get()
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	config.Server.Tags[0] = "mutated"
	config.Server.TLS.Enabled = false
	config.Labels["region"] = "west"

	current, _, err := configStore.Get()
	if err != nil {
		t.Fatalf("second Get() error = %v", err)
	}
	if current.Server.Tags[0] != "api" {
		t.Fatalf("detached tag = %q, want api", current.Server.Tags[0])
	}
	if !current.Server.TLS.Enabled {
		t.Fatal("detached TLS.Enabled = false, want true")
	}
	if current.Labels["region"] != "east" {
		t.Fatalf("detached region = %q, want east", current.Labels["region"])
	}
}

func TestWatchPollingReloadsExternalChanges(t *testing.T) {
	t.Parallel()

	provider := providertesting.New([]byte(`{"server":{"port":8080}}`))
	configStore, err := sundial.New[testConfig](
		context.Background(),
		provider,
		sundial.WithWatchInterval(5*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	changed := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		done <- configStore.Watch(ctx, sundial.WithOnChange(func() {
			changed <- struct{}{}
		}))
	}()

	provider.SetData([]byte(`{"server":{"port":9090}}`))
	select {
	case <-changed:
	case <-time.After(time.Second):
		t.Fatal("Watch did not report external change")
	}
	config, _, err := configStore.Get()
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if config.Server.Port != 9090 {
		t.Fatalf("watched port = %d, want 9090", config.Server.Port)
	}

	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Watch() error = %v, want context.Canceled", err)
	}
}

func TestNativeWatchReloadsExternalChanges(t *testing.T) {
	t.Parallel()

	provider := providertesting.NewWatcher([]byte(`{"enabled":false}`))
	configStore, err := sundial.New[testConfig](
		context.Background(),
		provider,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	changed := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		done <- configStore.Watch(ctx, sundial.WithOnChange(func() {
			changed <- struct{}{}
		}))
	}()

	provider.Change([]byte(`{"enabled":true}`))
	select {
	case <-changed:
	case <-time.After(time.Second):
		t.Fatal("native Watch did not report external change")
	}
	config, _, err := configStore.Get()
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !config.Enabled {
		t.Fatal("watched Enabled = false, want true")
	}

	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Watch() error = %v, want context.Canceled", err)
	}
}

func TestPutRejectsStaleVersionWithinInstance(t *testing.T) {
	t.Parallel()

	provider := providertesting.New([]byte(`{"counter":0,"enabled":false}`))
	configStore, err := sundial.New[testConfig](context.Background(), provider)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	first, firstVersion, err := configStore.Get()
	if err != nil {
		t.Fatalf("first Get() error = %v", err)
	}
	stale, staleVersion, err := configStore.Get()
	if err != nil {
		t.Fatalf("second Get() error = %v", err)
	}

	first.Counter = 1
	if putErr := configStore.Put(context.Background(), first, firstVersion); putErr != nil {
		t.Fatalf("first Put() error = %v", putErr)
	}
	stale.Enabled = true
	if putErr := configStore.Put(context.Background(), stale, staleVersion); !errors.Is(putErr, sundial.ErrConflict) {
		t.Fatalf("stale Put() error = %v, want ErrConflict", putErr)
	}

	current, _, err := configStore.Get()
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if current.Counter != 1 || current.Enabled {
		t.Fatalf("configuration after conflict = %#v, want first write only", current)
	}
}

func TestPutRejectsStaleVersionAcrossInstancesAndAllowsRetry(t *testing.T) {
	t.Parallel()

	provider := providertesting.New([]byte(`{"counter":0,"enabled":false}`))
	firstStore, err := sundial.New[testConfig](context.Background(), provider)
	if err != nil {
		t.Fatalf("first New() error = %v", err)
	}
	secondStore, err := sundial.New[testConfig](context.Background(), provider)
	if err != nil {
		t.Fatalf("second New() error = %v", err)
	}

	first, firstVersion, err := firstStore.Get()
	if err != nil {
		t.Fatalf("first Get() error = %v", err)
	}
	second, secondVersion, err := secondStore.Get()
	if err != nil {
		t.Fatalf("second Get() error = %v", err)
	}

	first.Counter = 1
	if putErr := firstStore.Put(context.Background(), first, firstVersion); putErr != nil {
		t.Fatalf("first Put() error = %v", putErr)
	}
	second.Enabled = true
	if putErr := secondStore.Put(context.Background(), second, secondVersion); !errors.Is(putErr, sundial.ErrConflict) {
		t.Fatalf("stale Put() error = %v, want ErrConflict", putErr)
	}

	if reloadErr := secondStore.Reload(context.Background()); reloadErr != nil {
		t.Fatalf("Reload() error = %v", reloadErr)
	}
	second, secondVersion, err = secondStore.Get()
	if err != nil {
		t.Fatalf("retry Get() error = %v", err)
	}
	second.Enabled = true
	if putErr := secondStore.Put(context.Background(), second, secondVersion); putErr != nil {
		t.Fatalf("retry Put() error = %v", putErr)
	}

	var saved testConfig
	if decodeErr := json.Unmarshal(provider.Data(), &saved); decodeErr != nil {
		t.Fatalf("decode saved configuration: %v", decodeErr)
	}
	if saved.Counter != 1 || !saved.Enabled {
		t.Fatalf("configuration after retry = %#v, want both changes", saved)
	}
}

func TestPutAllowsOnlyOneConcurrentCreate(t *testing.T) {
	t.Parallel()

	provider := providertesting.New(nil)
	firstStore, err := sundial.New[testConfig](context.Background(), provider)
	if err != nil {
		t.Fatalf("first New() error = %v", err)
	}
	secondStore, err := sundial.New[testConfig](context.Background(), provider)
	if err != nil {
		t.Fatalf("second New() error = %v", err)
	}

	first, firstVersion, err := firstStore.Get()
	if err != nil {
		t.Fatalf("first Get() error = %v", err)
	}
	second, secondVersion, err := secondStore.Get()
	if err != nil {
		t.Fatalf("second Get() error = %v", err)
	}

	first.Counter = 1
	if putErr := firstStore.Put(context.Background(), first, firstVersion); putErr != nil {
		t.Fatalf("first Put() error = %v", putErr)
	}
	second.Enabled = true
	if putErr := secondStore.Put(context.Background(), second, secondVersion); !errors.Is(putErr, sundial.ErrConflict) {
		t.Fatalf("second Put() error = %v, want ErrConflict", putErr)
	}

	var saved testConfig
	if decodeErr := json.Unmarshal(provider.Data(), &saved); decodeErr != nil {
		t.Fatalf("decode saved configuration: %v", decodeErr)
	}
	if saved.Counter != 1 || saved.Enabled {
		t.Fatalf("saved configuration = %#v, want first write only", saved)
	}
}

func TestReloadTracksChangedProviderVersionWhenContentIsUnchanged(t *testing.T) {
	t.Parallel()

	data := []byte(`{"enabled":true}`)
	provider := providertesting.New(data)
	configStore, err := sundial.New[testConfig](context.Background(), provider)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	provider.SetData(data)
	if reloadErr := configStore.Reload(context.Background()); reloadErr != nil {
		t.Fatalf("Reload() error = %v", reloadErr)
	}
	config, version, err := configStore.Get()
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	config.Enabled = false
	if putErr := configStore.Put(context.Background(), config, version); putErr != nil {
		t.Fatalf("Put() error = %v", putErr)
	}
	if got := provider.SaveCount(); got != 1 {
		t.Fatalf("SaveCount() = %d, want 1 without a stale-version retry", got)
	}
}

func TestConcurrentReadsAndWrites(t *testing.T) {
	t.Parallel()

	provider := providertesting.New([]byte(`{"counter":0}`))
	configStore, err := sundial.New[testConfig](
		context.Background(),
		provider,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	var group sync.WaitGroup
	for i := range 20 {
		group.Add(2)
		go func(value int) {
			defer group.Done()
			for {
				config, version, getErr := configStore.Get()
				if getErr != nil {
					t.Errorf("Get() error = %v", getErr)
					return
				}
				config.Counter = value
				putErr := configStore.Put(context.Background(), config, version)
				if errors.Is(putErr, sundial.ErrConflict) {
					continue
				}
				if putErr != nil {
					t.Errorf("Put() error = %v", putErr)
				}
				return
			}
		}(i)
		go func() {
			defer group.Done()
			if _, _, getErr := configStore.Get(); getErr != nil {
				t.Errorf("Get() error = %v", getErr)
			}
		}()
	}
	group.Wait()
}
