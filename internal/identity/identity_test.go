package identity

import "testing"

func TestAutoMonogram(t *testing.T) {
	taken := map[string]bool{}
	for _, c := range []struct{ name, want string }{
		{"initech-admin", "IA"}, {"initech-readonly", "IR"}, {"acme", "AC"}, {"globex", "GL"}, {"web-admin", "WA"},
	} {
		got := AutoMonogram(c.name, taken)
		if got != c.want {
			t.Errorf("%s: got %s want %s", c.name, got, c.want)
		}
		taken[got] = true
	}
	// digits when every letter pair is taken
	taken = map[string]bool{"AB": true}
	if got := AutoMonogram("a-b", taken); got != "A2" {
		t.Fatal(got)
	}
	if got := AutoMonogram("---", map[string]bool{}); len(got) != 2 {
		t.Fatal(got)
	}
	if got := AutoMonogram("数据_库", map[string]bool{}); got != "数库" {
		t.Fatal(got)
	}
}

func TestAssignOrder(t *testing.T) {
	in := []Input{{"acme", ""}, {"globex", ""}, {"initech-admin", "777788889999"}, {"initech-readonly", "777788889999"}}
	res, colors := Assign(in, map[string]int{}, nil)
	want := map[string]int{"acme": 1, "globex": 3, "initech-admin": 2, "initech-readonly": 2}
	for k, v := range want {
		if res[k].Slot != v {
			t.Errorf("%s slot %d want %d", k, res[k].Slot, v)
		}
	}
	if colors["profile:acme"] != 1 || colors["777788889999"] != 2 {
		t.Fatalf("%v", colors)
	}
	if res["initech-admin"].Monogram != "IA" || res["initech-readonly"].Monogram != "IR" || res["acme"].Monogram != "AC" || res["globex"].Monogram != "GL" {
		t.Fatalf("%v", res)
	}
}

func TestRekeyKeepsSlot(t *testing.T) {
	colors := map[string]int{"profile:acme": 6}
	res, nc := Assign([]Input{{"acme", "111122223333"}}, colors, nil)
	if res["acme"].Slot != 6 || nc["111122223333"] != 6 {
		t.Fatalf("%v %v", res, nc)
	}
	if _, ok := nc["profile:acme"]; ok {
		t.Fatal("old key must be removed")
	}
	if colors["profile:acme"] != 6 || len(colors) != 1 {
		t.Fatal("input map must not be modified")
	}
	// existing account colour wins over a stale profile key
	res, nc = Assign([]Input{{"x", "1"}}, map[string]int{"1": 4, "profile:x": 5}, nil)
	if res["x"].Slot != 4 {
		t.Fatalf("%v", res)
	}
	if _, ok := nc["profile:x"]; ok {
		t.Fatal("stale key kept")
	}
}

func TestRubyNeverAuto(t *testing.T) {
	var in []Input
	for i := 0; i < 20; i++ {
		in = append(in, Input{Profile: "p" + string(rune('a'+i)), AccountID: string(rune('A'+i)) + "00000000000"})
	}
	res, _ := Assign(in, map[string]int{}, nil)
	seen := map[int]int{}
	for _, r := range res {
		if r.Slot < 1 || r.Slot > 7 {
			t.Fatalf("slot %d out of automatic range", r.Slot)
		}
		seen[r.Slot]++
	}
	if len(seen) != 7 {
		t.Fatalf("expected all 7 automatic slots in use, got %v", seen)
	}
	// the user can still choose 8
	res, _ = Assign([]Input{{"a", "1"}}, map[string]int{"1": 8}, nil)
	if res["a"].Slot != 8 {
		t.Fatal("user choice of 8 must be kept")
	}
}

func TestMonogramOverrides(t *testing.T) {
	res, _ := Assign([]Input{{"initech-admin", "1"}, {"web-main", "2"}}, nil, map[string]string{"initech-admin": "xy", "web-main": "bad!"})
	if res["initech-admin"].Monogram != "XY" || res["web-main"].Monogram != "WM" {
		t.Fatalf("%v", res)
	}
}

func TestValidMonogram(t *testing.T) {
	for _, ok := range []string{"AB", "a1", "09", "云"} {
		if !ValidMonogram(ok) {
			t.Errorf("%q should be valid", ok)
		}
	}
	for _, bad := range []string{"", "A", "ABC", "A-", "云云", "é1", "a b", "α"} {
		if ValidMonogram(bad) {
			t.Errorf("%q should be invalid", bad)
		}
	}
}
