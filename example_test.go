package sundial_test

import (
	"context"
	"fmt"

	"github.com/sundayfun/sundial"
	providertesting "github.com/sundayfun/sundial/provider/testing"
)

func ExampleNew() {
	type Config struct {
		Port int `json:"port"`
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Stop automatic reloads when the client is no longer needed.

	// Use an in-memory test provider with an existing JSON document.
	provider := providertesting.New([]byte(`{"port":8080}`))
	client, err := sundial.New[Config](ctx, provider)
	if err != nil {
		panic(err)
	}
	entry := client.Get()
	fmt.Println(entry.Value.Port)

	// Output: 8080
}

func ExampleClient_Put() {
	type Config struct {
		Port int `json:"port"`
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	provider := providertesting.New([]byte(`{"port":8080}`))
	client, err := sundial.New[Config](ctx, provider)
	if err != nil {
		panic(err)
	}
	entry, err := client.Draft()
	if err != nil {
		panic(err)
	}

	// Keep the observed revision while changing the complete document's value.
	entry.Value.Port = 9090
	saved, err := client.Put(ctx, entry)
	if err != nil {
		panic(err)
	}
	current := client.Get()
	fmt.Println(saved.Value.Port)
	fmt.Println(saved.Revision.ID != entry.Revision.ID)
	fmt.Println(current.Revision.ID == saved.Revision.ID)

	// Output:
	// 9090
	// true
	// true
}

func ExampleClient_Put_conflict() {
	type Config struct {
		Port int `json:"port"`
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	provider := providertesting.New([]byte(`{"port":8080}`))
	client, err := sundial.New[Config](ctx, provider)
	if err != nil {
		panic(err)
	}
	entry, err := client.Draft()
	if err != nil {
		panic(err)
	}
	stale, err := client.Draft()
	if err != nil {
		panic(err)
	}
	entry.Value.Port = 9090
	if _, err = client.Put(ctx, entry); err != nil {
		panic(err)
	}

	// A second write based on the old revision must not overwrite the first.
	stale.Value.Port = 7070
	_, err = client.Put(ctx, stale)
	fmt.Println(sundial.IsConflict(err))
	current := client.Get()
	fmt.Println(current.Value.Port)

	// Output:
	// true
	// 9090
}

func ExampleClient_Draft() {
	type Config struct {
		Labels map[string]string `json:"labels"`
		Tags   []string          `json:"tags"`
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	provider := providertesting.New([]byte(`{"labels":{"region":"east"},"tags":["api"]}`))
	client, err := sundial.New[Config](ctx, provider)
	if err != nil {
		panic(err)
	}
	entry, err := client.Draft()
	if err != nil {
		panic(err)
	}
	entry.Value.Labels["region"] = "west"
	entry.Value.Tags[0] = "worker"
	current := client.Get()
	fmt.Println(current.Value.Labels["region"], current.Value.Tags[0])
	// Output: east api
}

func ExampleClient_Update() {
	type Config struct {
		Labels map[string]string `json:"labels"`
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	provider := providertesting.New([]byte(`{"labels":{"region":"east"}}`))
	client, err := sundial.New[Config](ctx, provider)
	if err != nil {
		panic(err)
	}

	// Edit an independent draft without first calling Get.
	saved, err := client.Update(ctx, func(config *Config) error {
		config.Labels["region"] = "west"
		return nil
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(saved.Value.Labels["region"])
	fmt.Println(client.Get().Value.Labels["region"])

	// Output:
	// west
	// west
}
