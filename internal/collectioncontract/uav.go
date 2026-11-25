package collectioncontract

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// UAVSearchRequest 仅接受固定站点、有界关键词和页码。
type UAVSearchRequest struct {
	Site  string `json:"site"`  // 固定 jable、missav、hanime1、huangguo 或 supjav。
	Query string `json:"query"` // 1–120 个字符的搜索关键词。
	Page  int    `json:"page"`  // 从1开始的结果页，最大1000。
}

// UAVSearchItem 是不含播放链接、缩略图或凭据的视频元信息。
type UAVSearchItem struct {
	VideoID  string `json:"videoId"`  // 站点内规范视频编号。
	Title    string `json:"title"`    // 纯文本展示标题。
	Duration string `json:"duration"` // 上游时长文本，未知时为空。
}

// UAVSearchResponse 保存有界结果页，不伪造上游未知总数。
type UAVSearchResponse struct {
	Site    string          `json:"site"`    // 原请求的固定站点。
	Query   string          `json:"query"`   // 已规范化的关键词。
	Page    int             `json:"page"`    // 当前结果页。
	HasNext bool            `json:"hasNext"` // 上游分页是否含下一页。
	Items   []UAVSearchItem `json:"items"`   // 最多100个结果，空结果为数组。
}

// uavSlug 限制视频编号不能携带路径或URL语义。
var uavSlug = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,199}$`)

// uavHuangguoID 将作品与分集一起绑定，不允许省略集号或下载整剧。
var uavHuangguoID = regexp.MustCompile(`^huangguo-([1-9][0-9]{0,18})-ep-([1-9][0-9]{0,9})$`)

// ValidUAVSite 只接受已明确审核的固定站点。
func ValidUAVSite(site string) bool {
	return site == "jable" || site == "missav" || site == "hanime1" || site == "huangguo" || site == "supjav"
}

// ValidUAVVideoID 在主服务与执行端共用严格站点编号规则。
func ValidUAVVideoID(site string, id string) bool {
	if site == "huangguo" {
		parts := uavHuangguoID.FindStringSubmatch(id)
		if len(parts) != 3 {
			return false
		}
		_, seriesErr := strconv.ParseUint(parts[1], 10, 63)
		_, episodeErr := strconv.ParseUint(parts[2], 10, 31)
		return seriesErr == nil && episodeErr == nil
	}
	if site == "hanime1" || site == "supjav" {
		return ValidJMComicAlbumID(id)
	}
	if !uavSlug.MatchString(id) {
		return false
	}
	if site == "jable" {
		return id == strings.ToLower(id)
	}
	if site != "missav" {
		return false
	}
	hasDigit := false
	for _, ch := range id {
		if ch >= '0' && ch <= '9' {
			hasDigit = true
		}
	}
	if strings.HasPrefix(strings.ToLower(id), "dm") && len(id) > 2 {
		onlyDigits := true
		for _, ch := range id[2:] {
			if ch < '0' || ch > '9' {
				onlyDigits = false
			}
		}
		if onlyDigits {
			return false
		}
	}
	return hasDigit
}

// NormalizeUAVSearch 规范化搜索并拒绝控制字符及越界页码。
func NormalizeUAVSearch(input UAVSearchRequest) (UAVSearchRequest, error) {
	input.Query = strings.TrimSpace(input.Query)
	if !ValidUAVSite(input.Site) {
		return input, fmt.Errorf("视频搜索站点不受支持")
	}
	if !utf8.ValidString(input.Query) || utf8.RuneCountInString(input.Query) < 1 || utf8.RuneCountInString(input.Query) > 120 {
		return input, fmt.Errorf("视频搜索关键词需要1–120个字符")
	}
	for _, ch := range input.Query {
		if unicode.IsControl(ch) {
			return input, fmt.Errorf("视频搜索关键词不能包含控制字符")
		}
	}
	if input.Page == 0 {
		input.Page = 1
	}
	if input.Page < 1 || input.Page > 1000 {
		return input, fmt.Errorf("视频搜索页码需要在1–1000之间")
	}
	return input, nil
}

// ValidateUAVSearchResponse 独立验证桥接的来源、条数和文本边界。
func ValidateUAVSearchResponse(input UAVSearchRequest, output UAVSearchResponse) error {
	if output.Site != input.Site || output.Query != input.Query || output.Page != input.Page || len(output.Items) > 100 {
		return fmt.Errorf("视频搜索响应超过允许范围")
	}
	seen := map[string]bool{}
	for _, item := range output.Items {
		if !ValidUAVVideoID(input.Site, item.VideoID) || seen[item.VideoID] || !utf8.ValidString(item.Title) || len(item.Title) > 1200 || len(item.Duration) > 40 || !utf8.ValidString(item.Duration) {
			return fmt.Errorf("视频搜索返回无效编号或文本")
		}
		for _, ch := range item.Title + item.Duration {
			if unicode.IsControl(ch) {
				return fmt.Errorf("视频搜索返回无效控制字符")
			}
		}
		seen[item.VideoID] = true
	}
	return nil
}
