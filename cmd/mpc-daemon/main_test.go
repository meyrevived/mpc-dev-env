package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fsnotify/fsnotify"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMain(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Main Daemon Suite")
}

var _ = Describe("Main Daemon File Watcher", func() {
	Describe("shouldIgnoreEvent", func() {
		It("should ignore non-write and non-create events", func() {
			Expect(shouldIgnoreEvent(fsnotify.Event{Name: "test.go", Op: fsnotify.Remove})).To(BeTrue())
			Expect(shouldIgnoreEvent(fsnotify.Event{Name: "test.go", Op: fsnotify.Rename})).To(BeTrue())
			Expect(shouldIgnoreEvent(fsnotify.Event{Name: "test.go", Op: fsnotify.Chmod})).To(BeTrue())
		})

		It("should not ignore write or create events for regular files", func() {
			Expect(shouldIgnoreEvent(fsnotify.Event{Name: "test.go", Op: fsnotify.Write})).To(BeFalse())
			Expect(shouldIgnoreEvent(fsnotify.Event{Name: "path/to/file.txt", Op: fsnotify.Create})).To(BeFalse())
		})

		It("should ignore temporary file extensions", func() {
			for _, ext := range []string{".swp", ".swo", ".pyc", ".pyo", ".log", ".tmp"} {
				event := fsnotify.Event{Name: "file" + ext, Op: fsnotify.Write}
				Expect(shouldIgnoreEvent(event)).To(BeTrue(), "Failed for extension "+ext)
			}
		})

		It("should ignore hidden files", func() {
			event := fsnotify.Event{Name: ".test.go", Op: fsnotify.Write}
			Expect(shouldIgnoreEvent(event)).To(BeTrue())
			event = fsnotify.Event{Name: "path/to/.env", Op: fsnotify.Write}
			Expect(shouldIgnoreEvent(event)).To(BeTrue())
		})
	})

	Describe("addRecursiveWatch", func() {
		var (
			watcher *fsnotify.Watcher
			tempDir string
		)

		BeforeEach(func() {
			var err error
			watcher, err = fsnotify.NewWatcher()
			Expect(err).NotTo(HaveOccurred())

			tempDir, err = os.MkdirTemp("", "watch-test-*")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			_ = watcher.Close()
			_ = os.RemoveAll(tempDir)
		})

		It("should add directories recursively", func() {
			dir1 := filepath.Join(tempDir, "dir1")
			dir2 := filepath.Join(tempDir, "dir1", "dir2")
			Expect(os.MkdirAll(dir2, 0755)).To(Succeed())

			err := addRecursiveWatch(watcher, tempDir)
			Expect(err).NotTo(HaveOccurred())

			watchList := watcher.WatchList()
			Expect(watchList).To(ContainElement(tempDir))
			Expect(watchList).To(ContainElement(dir1))
			Expect(watchList).To(ContainElement(dir2))
		})

		It("should skip ignored directories", func() {
			gitDir := filepath.Join(tempDir, ".git")
			ideaDir := filepath.Join(tempDir, ".idea")
			nodeModulesDir := filepath.Join(tempDir, "node_modules")
			subdir := filepath.Join(tempDir, "subdir")

			Expect(os.MkdirAll(gitDir, 0755)).To(Succeed())
			Expect(os.MkdirAll(ideaDir, 0755)).To(Succeed())
			Expect(os.MkdirAll(nodeModulesDir, 0755)).To(Succeed())
			Expect(os.MkdirAll(subdir, 0755)).To(Succeed())

			err := addRecursiveWatch(watcher, tempDir)
			Expect(err).NotTo(HaveOccurred())

			watchList := watcher.WatchList()
			Expect(watchList).To(ContainElement(tempDir))
			Expect(watchList).To(ContainElement(subdir))
			Expect(watchList).NotTo(ContainElement(gitDir))
			Expect(watchList).NotTo(ContainElement(ideaDir))
			Expect(watchList).NotTo(ContainElement(nodeModulesDir))
		})
	})
})
