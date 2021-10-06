package reddit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/vartanbeno/go-reddit/v2/reddit"
)

func getData() []*reddit.Post {
	var data []*reddit.Post
	base := time.Now()
	for d := 0; d < 28; d++ {
		t := &reddit.Timestamp{Time: base.Add(24 * time.Hour * time.Duration(d))}
		data = append(data, &reddit.Post{Created: t})
	}
	return data
}

func TestCreateListInOrder(t *testing.T) {
	data := getData()
	rm := newRollingMedian(data)
	assert.NotNil(t, rm)

	assert.NotNil(t, rm.head)
	assert.NotNil(t, rm.tail)

	// value are already sorted, they should be in the same order
	var prev *listItem
	curr := rm.head
	for k := 0; k < rm.len; k++ {
		assert.NotNil(t, curr)
		assert.Same(t, curr.value, data[k])
		prev = curr
		curr = curr.next
	}
	assert.Same(t, prev, rm.tail)
	assert.Nil(t, curr)

	assert.Equal(t, rm.cachedValue, rm.cacheValue())
}

func TestCreateListInReverseOrder(t *testing.T) {
	data := getData()
	// reverse data
	for i, j := 0, len(data)-1; i < j; i, j = i+1, j-1 {
		data[i], data[j] = data[j], data[i]
	}

	rm := newRollingMedian(data)
	assert.NotNil(t, rm)

	assert.NotNil(t, rm.head)
	assert.NotNil(t, rm.tail)

	// value are already sorted, they should be in the same order
	var prev *listItem
	curr := rm.head
	for k := 0; k < rm.len; k++ {
		assert.NotNil(t, curr)
		assert.Same(t, curr.value, data[rm.len-k-1])
		prev = curr
		curr = curr.next
	}
	assert.Same(t, prev, rm.tail)
	assert.Nil(t, curr)

	assert.Equal(t, rm.cachedValue, rm.cacheValue())
}

// TODO random order

// TODO check for add
