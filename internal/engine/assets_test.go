package engine

import "testing"

func TestAssetEnv(t *testing.T) {
	m, err := assetEnv([]byte(`{"id":"a","originalFileName":"SERVICE-A_1_cat.Dog.JPG","type":"IMAGE"}`))
	if err != nil {
		t.Fatal(err)
	}
	if m["baseName"] != "SERVICE-A_1_cat.Dog" || m["extension"] != "jpg" || m["type"] != "IMAGE" {
		t.Errorf("%v", m)
	}
}
