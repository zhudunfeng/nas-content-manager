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
	return nil
}
