package cloud

import "testing"

func TestSpecResolutionValidationAndActionInputs(t *testing.T) {
	p := Property{IID: 1, Type: "urn:miot-spec-v2:property:on:1", Format: "bool", Access: []string{"read", "write"}}
	s := Spec{Services: []Service{{IID: 2, Properties: []Property{p}, Actions: []Action{{IID: 1, Type: "urn:miot-spec-v2:action:switch:1", In: []int{1}}}}, {IID: 3, Properties: []Property{p}}}}
	if _, _, e := s.Property("on"); e == nil {
		t.Fatal("ambiguous property resolved silently")
	}
	siid, got, e := s.Property("2.1")
	if e != nil || siid != 2 || got.Validate(true) != nil || got.Validate("true") == nil {
		t.Fatal("property validation failed")
	}
	svc, a, e := s.Action("switch")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = svc.Inputs(a, []any{true}); e != nil {
		t.Fatal(e)
	}
	if _, e = svc.Inputs(a, []any{}); e == nil {
		t.Fatal("accepted missing action inputs")
	}
	n := Property{Format: "uint8", Range: []float64{0, 100, 5}}
	for _, v := range []float64{-1, 101, 256, 2.5, 3} {
		if n.Validate(v) == nil {
			t.Fatalf("accepted invalid value %v", v)
		}
	}
	if n.Validate(float64(50)) != nil {
		t.Fatal("rejected valid range/step")
	}
}
