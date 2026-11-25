package collectioncontract

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ValidJMComicAlbumID 拒绝零、符号、前导零和任何非数字的漫画编号。
func ValidJMComicAlbumID(value string) bool {
	if len(value) < 1 || len(value) > 12 || value[0] < '1' || value[0] > '9' {
		return false
	}
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

// NormalizeJMComicSearch 在两端共用同一关键词和页码边界。
func NormalizeJMComicSearch(input *JMComicSearchRequest) error {
	input.Query = strings.TrimSpace(input.Query)
	if !utf8.ValidString(input.Query) || utf8.RuneCountInString(input.Query) < 1 || utf8.RuneCountInString(input.Query) > 120 {
		return fmt.Errorf("漫画搜索关键词需要1–120个字符")
	}
	for _, ch := range input.Query {
		if unicode.IsControl(ch) {
			return fmt.Errorf("漫画搜索关键词不能包含控制字符")
		}
	}
	if input.Page == 0 {
		input.Page = 1
	}
	if input.Page < 1 || input.Page > 1000 {
		return fmt.Errorf("漫画搜索页码需要在1–1000之间")
	}
	if input.OrderBy != "" && input.OrderBy != "mr" && input.OrderBy != "mv" && input.OrderBy != "mp" && input.OrderBy != "tf" {
		return fmt.Errorf("漫画搜索排序不受支持")
	}
	if input.Time != "" && input.Time != "a" && input.Time != "t" && input.Time != "w" && input.Time != "m" {
		return fmt.Errorf("漫画搜索时间范围不受支持")
	}
	return nil
}

// ValidateJMComicChapters 拒绝无效或重复章节，空数组保留全本语义。
func ValidateJMComicChapters(ids []string) error {
	if len(ids) > 10000 {
		return fmt.Errorf("漫画章节选择超过限制")
	}
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if !ValidJMComicAlbumID(id) || seen[id] {
			return fmt.Errorf("漫画章节编号无效或重复")
		}
		seen[id] = true
	}
	return nil
}
