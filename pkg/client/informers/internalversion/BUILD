load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "factory.go",
        "generic.go",
    ],
    importpath = "kubevirt.io/kubevirt/pkg/client/informers/internalversion",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/k8s.io/apimachinery/pkg/runtime:go_default_library",
        "//vendor/k8s.io/apimachinery/pkg/runtime/schema:go_default_library",
        "//vendor/k8s.io/client-go/tools/cache:go_default_library",
        "//vendor/kubevirt.io/kubevirt/pkg/apis/wardle:go_default_library",
        "//vendor/kubevirt.io/kubevirt/pkg/client/clientset/internalversion:go_default_library",
        "//vendor/kubevirt.io/kubevirt/pkg/client/informers/internalversion/internalinterfaces:go_default_library",
        "//vendor/kubevirt.io/kubevirt/pkg/client/informers/internalversion/wardle:go_default_library",
    ],
)

filegroup(
    name = "package-srcs",
    srcs = glob(["**"]),
    tags = ["automanaged"],
    visibility = ["//visibility:private"],
)

filegroup(
    name = "all-srcs",
    srcs = [
        ":package-srcs",
        "//staging/src/kubevirt.io/kubevirt/pkg/client/informers/internalversion/internalinterfaces:all-srcs",
        "//staging/src/kubevirt.io/kubevirt/pkg/client/informers/internalversion/wardle:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
