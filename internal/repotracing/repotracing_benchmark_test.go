package repotracing_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kopia/kopia/internal/contentparam"
	"github.com/kopia/kopia/internal/repotracing"
	"github.com/kopia/kopia/internal/repotracing/logparam"
	"github.com/kopia/kopia/repo/content/index"
)

func BenchmarkLogger(b *testing.B) {
	ctx := context.Background()

	cid, err := index.ParseID("1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef")
	require.NoError(b, err)

	// context params
	ctx = repotracing.WithParams(ctx,
		logparam.String("service", "test-service"),
		logparam.Int("version", 2),
		contentparam.ContentID("cid", cid),
	)

	// logger params
	l := repotracing.NewLogger(func(data []byte) {},
		logparam.String("lservice", "test-service"),
	)

	for b.Loop() {
		repotracing.Log(ctx, l, "baz")
		repotracing.Log1(ctx, l, "baz", logparam.String("arg1", "123\x01foobar"))
		repotracing.Log2(ctx, l, "baz", logparam.Int("arg1", 123), logparam.Int("arg2", 456))
		repotracing.Log3(ctx, l, "baz", logparam.Int("arg1", 123), logparam.Int("arg2", 456), logparam.Int("arg3", 789))
		repotracing.Log4(ctx, l, "baz", logparam.Int("arg1", 123), logparam.Int("arg2", 456), logparam.Int("arg3", 789), logparam.Int("arg4", 101112))
		repotracing.Log5(ctx, l, "baz", logparam.Int("arg1", 123), logparam.Int("arg2", 456), logparam.Int("arg3", 789), logparam.Int("arg4", 101112), logparam.Int("arg5", 123456))
		repotracing.Log6(ctx, l, "baz", logparam.Int("arg1", 123), logparam.Int("arg2", 456), logparam.Int("arg3", 789), logparam.Int("arg4", 101112), logparam.Int("arg5", 123456), logparam.Int("arg6", 123456))
	}
}
