package app

import "testing"

func TestSeededResourceNameKeysPreserveCustomNames(t *testing.T) {
	s, _, c := fixture(t)
	tree := decodeValue[struct{ Items []FileEntry }](t, c.call("GET", "/files/tree", nil, 200))
	if len(tree.Items) != 2 || tree.Items[0].NameKey != "全部文件" || tree.Items[1].NameKey != "未分类" || tree.Items[1].CreatorKey != "系统" {
		t.Fatalf("default folders: %+v", tree.Items)
	}
	roles := decodeValue[[]Role](t, c.call("GET", "/roles", nil, 200))
	for _, role := range roles {
		if role.NameKey == "" || role.DescriptionKey == "" {
			t.Fatalf("default role: %+v", role)
		}
	}
	styles := decodeValue[[]ImageStyle](t, c.call("GET", "/image-styles", nil, 200))
	if len(styles) != 2 || styles[0].NameKey != "列表缩略图" || styles[1].NameKey != "文档插图" {
		t.Fatalf("default styles: %+v", styles)
	}
	// Same text explicitly saved by a user becomes literal, not a translation key.
	c.call("PATCH", "/files/2", map[string]any{"name": "未分类", "parentId": "1", "version": 1}, 200)
	tree = decodeValue[struct{ Items []FileEntry }](t, c.call("GET", "/files/tree", nil, 200))
	if tree.Items[1].NameKey != "" || tree.Items[1].Name != "未分类" {
		t.Fatal("renamed default translated")
	}
	custom := folderForTest(t, c, "1", "列表缩略图")
	entry, err := readFile(s.Store.DB, custom)
	if err != nil || entry.NameKey != "" {
		t.Fatalf("custom folder: %+v %v", entry, err)
	}
	c.call("PATCH", "/image-styles/1", ImageStyle{Name: "列表缩略图", Width: 320, Height: 240, Fit: "cover", Version: 1}, 200)
	styles = decodeValue[[]ImageStyle](t, c.call("GET", "/image-styles", nil, 200))
	if styles[0].NameKey != "" {
		t.Fatal("edited style translated")
	}
	// A legacy custom role occupying a seeded ID is not translated by ID alone.
	mustSQL(t, s, "UPDATE roles SET name='自定义角色',description='自定义说明' WHERE id=2")
	roles = decodeValue[[]Role](t, c.call("GET", "/roles", nil, 200))
	if roles[1].NameKey != "" || roles[1].DescriptionKey != "" {
		t.Fatal("legacy custom role translated")
	}
	mustSQL(t, s, "INSERT INTO hosts(id,hostname,alias,source) VALUES(1,'localhost','本机','local'),(2,'remote','本机','agent')")
	hosts := decodeValue[[]Host](t, c.call("GET", "/hosts", nil, 200))
	if hosts[0].AliasKey != "本机" || hosts[1].AliasKey != "" {
		t.Fatalf("aliases: %+v", hosts)
	}
	c.call("PATCH", "/hosts/1", map[string]any{"alias": "本机", "version": 1}, 200)
	hosts = decodeValue[[]Host](t, c.call("GET", "/hosts", nil, 200))
	if hosts[0].AliasKey != "" {
		t.Fatal("custom host alias translated")
	}
}
