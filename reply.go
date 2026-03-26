package taskrunner

import (
	"time"

	core "github.com/Swind/go-task-runner/core"
)

func PostTaskAndReplyWithResult[T any](
	targetRunner TaskRunner,
	task TaskWithResult[T],
	reply ReplyWithResult[T],
	replyRunner TaskRunner,
) {
	core.PostTaskAndReplyWithResult(targetRunner, task, reply, replyRunner)
}

func PostTaskAndReplyWithResultAndTraits[T any](
	targetRunner TaskRunner,
	task TaskWithResult[T],
	taskTraits TaskTraits,
	reply ReplyWithResult[T],
	replyTraits TaskTraits,
	replyRunner TaskRunner,
) {
	core.PostTaskAndReplyWithResultAndTraits(targetRunner, task, taskTraits, reply, replyTraits, replyRunner)
}

func PostDelayedTaskAndReplyWithResult[T any](
	targetRunner TaskRunner,
	task TaskWithResult[T],
	delay time.Duration,
	reply ReplyWithResult[T],
	replyRunner TaskRunner,
) {
	core.PostDelayedTaskAndReplyWithResult(targetRunner, task, delay, reply, replyRunner)
}

func PostDelayedTaskAndReplyWithResultAndTraits[T any](
	targetRunner TaskRunner,
	task TaskWithResult[T],
	delay time.Duration,
	taskTraits TaskTraits,
	reply ReplyWithResult[T],
	replyTraits TaskTraits,
	replyRunner TaskRunner,
) {
	core.PostDelayedTaskAndReplyWithResultAndTraits(targetRunner, task, delay, taskTraits, reply, replyTraits, replyRunner)
}
