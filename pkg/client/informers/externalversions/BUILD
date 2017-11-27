load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "factory.go",
        "generic.go",
    ],
    importpath = "kubevirt.io/kubevirt/pkg/client/informers/externalversions",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/k8s.io/apimachinery/pkg/runtime:go_default_library",
        "//vendor/k8s.io/apimachinery/pkg/runtime/schema:go_default_library",
        "//vendor/k8s.io/client-go/tools/cache:go_default_library",
        "//vendor/kubevirt.io/kubevirt/pkg/apis/wardle/v1alpha1:go_default_library",
        "//vendor/kubevirt.io/kubevirt/pkg/client/clientset/versioned:go_default_library",
        "//vendor/kubevirt.io/kubevirt/pkg/client/informers/externalversions/internalinterfaces:go_default_library",
        "//vendor/kubevirt.io/kubevirt/pkg/client/informers/externalversions/wardle:go_default_library",
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
        "//staging/src/kubevirt.io/kubevirt/pkg/client/informers/externalversions/internalinterfaces:all-srcs",
        "//staging/src/kubevirt.io/kubevirt/pkg/client/informers/externalversions/wardle:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
