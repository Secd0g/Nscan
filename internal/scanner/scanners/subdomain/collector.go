package subdomain

import "context"

// collector 子域名收集器接口，每个数据源实现一个
type collector interface {
	Name() string
	Collect(ctx context.Context, domain string) ([]string, error)
}
