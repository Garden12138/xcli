package task

import (
	"errors"
	"fmt"
)

func (r *Runner) checkOriginal() error {
	if r.task.Workspace == nil {
		return nil
	}
	root := r.task.Workspace.Repository
	status, err := git(root, "status", "--porcelain")
	if err != nil {
		return err
	}
	head, err := git(root, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if len(status) != 0 || string(head) != r.task.Workspace.BaseCommit+"\n" {
		return errors.New("original repository changed during isolated task; stopping without reverting user data")
	}
	return nil
}

func (r *Runner) checkInputs() error {
	for _, a := range r.task.Inputs {
		data, err := boundedRead(a.Path)
		if err != nil {
			return err
		}
		if digest(data) != a.SHA256 {
			return fmt.Errorf("input changed during task: %s", a.Path)
		}
	}
	return nil
}

func (r *Runner) candidateAdapters() map[string]string {
	result := map[string]string{}
	for _, name := range r.task.Candidates {
		d, _ := r.Registry.Get(name)
		result[name] = d.Config.Adapter
	}
	return result
}
