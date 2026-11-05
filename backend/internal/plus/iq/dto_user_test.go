//go:build unit

package iq

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

// 用户端 DTO 的 JSON 字段白名单。
// 这些结构会原样发给普通用户。如果这个测试失败，说明有人往用户 DTO 里加了字段：
// 先确认新字段不包含账号、Key、错误、耗时、Token 等管理端信息，再更新这里的白名单。
func TestUserDTOFieldWhitelist(t *testing.T) {
	want := map[string][]string{
		"UserMeta":    {"enabled", "menu_name"},
		"UserGallery": {"groups"},
		"UserGroup":   {"id", "name", "platform", "tasks"},
		"UserTask":    {"model", "samples"},
		"UserSample":  {"created_at", "id", "preview_url"},
	}
	types := []any{UserMeta{}, UserGallery{}, UserGroup{}, UserTask{}, UserSample{}}
	for _, v := range types {
		rt := reflect.TypeOf(v)
		var got []string
		for i := 0; i < rt.NumField(); i++ {
			f := rt.Field(i)
			if f.Anonymous {
				t.Fatalf("%s 不允许嵌入其他结构（%s），嵌入会把对方的字段一起暴露给用户", rt.Name(), f.Name)
			}
			tag := strings.Split(f.Tag.Get("json"), ",")[0]
			if tag == "" || tag == "-" {
				t.Fatalf("%s.%s 必须显式声明 json 字段名", rt.Name(), f.Name)
			}
			got = append(got, tag)
		}
		sort.Strings(got)
		exp := append([]string{}, want[rt.Name()]...)
		sort.Strings(exp)
		if !reflect.DeepEqual(got, exp) {
			t.Fatalf("%s 字段变化：got %v, want %v", rt.Name(), got, exp)
		}
	}
}

// 用户端 DTO 不能引用管理端类型
func TestUserDTONoAdminTypes(t *testing.T) {
	forbidden := map[reflect.Type]bool{
		reflect.TypeOf(Run{}): true, reflect.TypeOf(RunView{}): true, reflect.TypeOf(AccountHop{}): true,
		reflect.TypeOf(HopView{}): true, reflect.TypeOf(Task{}): true, reflect.TypeOf(KeyOption{}): true,
	}
	var walk func(rt reflect.Type)
	seen := map[reflect.Type]bool{}
	walk = func(rt reflect.Type) {
		for rt.Kind() == reflect.Slice || rt.Kind() == reflect.Ptr || rt.Kind() == reflect.Map {
			rt = rt.Elem()
		}
		if rt.Kind() != reflect.Struct || seen[rt] {
			return
		}
		seen[rt] = true
		if forbidden[rt] {
			t.Fatalf("用户 DTO 引用了管理端类型 %s", rt.Name())
		}
		for i := 0; i < rt.NumField(); i++ {
			walk(rt.Field(i).Type)
		}
	}
	walk(reflect.TypeOf(UserGallery{}))
}
