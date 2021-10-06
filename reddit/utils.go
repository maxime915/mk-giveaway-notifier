package reddit

import (
	"time"

	"github.com/vartanbeno/go-reddit/v2/reddit"
)

func minOfAnchor(points []Position) time.Time {
	min := 0
	for i := 1; i < len(points); i++ {
		if points[i].Created.Time.Before(points[min].Created.Time) {
			min = i
		}
	}
	return points[min].Created.Time
}

// doubly linked list node
type listItem struct {
	value *reddit.Post
	prev  *listItem
	next  *listItem
}

// Rolling median
type rollingMedian struct {
	head, tail    *listItem
	len           int
	removalQueue  map[int]*listItem
	removalMinKey int
	cachedValue   time.Time
}

// create a rolling median from a slice of posts
func newRollingMedian(posts []*reddit.Post) *rollingMedian {
	rm := &rollingMedian{
		len:           len(posts),
		removalQueue:  make(map[int]*listItem, len(posts)),
		removalMinKey: 0,
		// cachedValue:  zero value
	}

	// build list

	// insert first node
	rm.head = &listItem{posts[0], nil, nil}
	rm.tail = rm.head
	rm.removalQueue[0] = rm.head

	// insert other nodes
	for i := 1; i < len(posts); i++ {
		node := rm.insertPost(posts[i])
		rm.removalQueue[i] = node
	}

	// build cache
	rm.cacheValue()

	return rm
}

func (rm *rollingMedian) insertPost(post *reddit.Post) *listItem {
	node := &listItem{post, nil, nil}

	// walk list to find position
	curr := rm.head
	for ; curr != nil; curr = curr.next {
		if post.Created.Time.Before(curr.value.Created.Time) {
			// insert before curr
			node.next = curr
			node.prev = curr.prev
			curr.prev = node

			if rm.head == curr {
				rm.head = node
			}
			break
		}
	}

	// not inserted
	if curr == nil {
		// insert after at the end
		rm.tail.next = node
		node.prev = rm.tail
		rm.tail = node
	}

	return node
}

// add a point, remove the oldest point, returns the current rolling median
func (rm *rollingMedian) add(post *reddit.Post) time.Time {
	// remove oldest node
	old := rm.removalQueue[rm.removalMinKey]
	if old.prev != nil {
		old.prev.next = old.next
	}
	if old.next != nil {
		old.next.prev = old.prev
	}
	if old == rm.head {
		rm.head = old.next
	}
	if old == rm.tail {
		rm.tail = old.prev
	}

	// add new node
	rm.removalQueue[rm.removalMinKey+rm.len] = rm.insertPost(post)
	rm.removalMinKey++

	return rm.cacheValue()
}

func (rm *rollingMedian) cacheValue() time.Time {
	curr := rm.head
	for i := 0; i < rm.len/2; i++ {
		curr = curr.next
	}

	rm.cachedValue = curr.value.Created.Time
	return rm.cachedValue
}
