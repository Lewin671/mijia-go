package cloud

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestInventoryPaginationAndSceneFlow(t *testing.T) {
	c := testClient(t)
	pages := 0
	sceneWrites := 0
	c.HTTP.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/app/v2/homeroom/gethome_merged":
			return response(`{"code":0,"result":{"homelist":[{"id":"101","uid":123,"name":"Synthetic"}]}}`), nil
		case "/app/home/home_device_list":
			pages++
			if pages == 1 {
				return response(`{"code":0,"result":{"device_info":[{"did":9007199254740993,"name":"A"}],"has_more":true,"max_did":"cursor"}}`), nil
			}
			return response(`{"code":0,"result":{"device_info":[{"did":"blt.synthetic","name":"B"}],"has_more":false}}`), nil
		case "/app/appgateway/miot/appsceneservice/AppSceneService/GetSimpleSceneList":
			return response(`{"code":0,"result":{"manual_scene_info_list":[{"scene_id":9007199254740993,"name":"Synthetic scene"}]}}`), nil
		case "/app/appgateway/miot/appsceneservice/AppSceneService/NewRunScene":
			sceneWrites++
			return response(`{"code":0,"result":{"accepted":true}}`), nil
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
			return nil, nil
		}
	})
	ds, e := c.Devices(context.Background(), "")
	if e != nil || len(ds) != 2 || ds[0].ID != "9007199254740993" || ds[1].HomeID != "101" {
		t.Fatalf("inventory or exact ID preservation failed: %v", e)
	}
	ss, e := c.Scenes(context.Background())
	if e != nil || len(ss) != 1 || ss[0].ID != "9007199254740993" {
		t.Fatal("scene decoding failed")
	}
	if _, e = c.RunScene(context.Background(), ss[0].ID, "missing"); e == nil || sceneWrites != 0 {
		t.Fatal("scene with nonexistent home executed")
	}
	if _, e = c.RunScene(context.Background(), ss[0].ID, "101"); e != nil || sceneWrites != 1 {
		t.Fatalf("scene call failed: %v", e)
	}
}

func TestRejectRepeatedPaginationCursor(t *testing.T) {
	c := testClient(t)
	pages := 0
	c.HTTP.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/app/v2/homeroom/gethome_merged" {
			return response(`{"code":0,"result":{"homelist":[{"id":"101","uid":123}]}}`), nil
		}
		pages++
		return response(`{"code":0,"result":{"device_info":[],"has_more":true,"max_did":"repeat"}}`), nil
	})
	if _, e := c.Devices(context.Background(), ""); e == nil || pages != 2 {
		t.Fatal("repeated pagination did not stop")
	}
}

func TestCloudIdentifiersDoNotAcceptNullOrRound(t *testing.T) {
	for _, raw := range []string{`null`, `1.5`, `1e5`, `-1`} {
		if _, e := wireID(json.RawMessage(raw)); e == nil {
			t.Fatalf("accepted identifier %s", raw)
		}
	}
}
