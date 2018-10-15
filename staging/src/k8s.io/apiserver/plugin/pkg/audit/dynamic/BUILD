load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = [
        "conversion.go",
        "dynamic.go",
    ],
    importmap = "k8s.io/kubernetes/vendor/k8s.io/apiserver/plugin/pkg/audit/dynamic",
    importpath = "k8s.io/apiserver/plugin/pkg/audit/dynamic",
    visibility = ["//visibility:public"],
    deps = [
        "//staging/src/k8s.io/api/auditregistration/v1alpha1:go_default_library",
        "//staging/src/k8s.io/apimachinery/pkg/labels:go_default_library",
        "//staging/src/k8s.io/apimachinery/pkg/types:go_default_library",
        "//staging/src/k8s.io/apimachinery/pkg/util/errors:go_default_library",
        "//staging/src/k8s.io/apiserver/pkg/apis/audit:go_default_library",
        "//staging/src/k8s.io/apiserver/pkg/audit:go_default_library",
        "//staging/src/k8s.io/apiserver/pkg/audit/policy:go_default_library",
        "//staging/src/k8s.io/apiserver/pkg/audit/util:go_default_library",
        "//staging/src/k8s.io/apiserver/pkg/util/webhook:go_default_library",
        "//staging/src/k8s.io/apiserver/plugin/pkg/audit/buffered:go_default_library",
        "//staging/src/k8s.io/apiserver/plugin/pkg/audit/enforced:go_default_library",
        "//staging/src/k8s.io/apiserver/plugin/pkg/audit/enforced/legacy:go_default_library",
        "//staging/src/k8s.io/apiserver/plugin/pkg/audit/truncate:go_default_library",
        "//staging/src/k8s.io/apiserver/plugin/pkg/audit/webhook:go_default_library",
        "//staging/src/k8s.io/client-go/informers:go_default_library",
        "//staging/src/k8s.io/client-go/listers/auditregistration/v1alpha1:go_default_library",
        "//staging/src/k8s.io/client-go/tools/cache:go_default_library",
        "//vendor/github.com/golang/glog:go_default_library",
    ],
)

go_test(
    name = "go_default_test",
    srcs = [
        "conversion_test.go",
        "dynamic_test.go",
    ],
    embed = [":go_default_library"],
    deps = [
        "//staging/src/k8s.io/api/auditregistration/v1alpha1:go_default_library",
        "//staging/src/k8s.io/apiserver/pkg/apis/audit:go_default_library",
        "//staging/src/k8s.io/apiserver/pkg/audit:go_default_library",
        "//staging/src/k8s.io/apiserver/pkg/audit/policy:go_default_library",
        "//staging/src/k8s.io/apiserver/pkg/util/webhook:go_default_library",
        "//staging/src/k8s.io/apiserver/plugin/pkg/audit/buffered:go_default_library",
        "//staging/src/k8s.io/apiserver/plugin/pkg/audit/fake:go_default_library",
        "//staging/src/k8s.io/apiserver/plugin/pkg/audit/truncate:go_default_library",
        "//staging/src/k8s.io/client-go/informers:go_default_library",
        "//staging/src/k8s.io/client-go/kubernetes/fake:go_default_library",
        "//vendor/github.com/stretchr/testify/require:go_default_library",
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
    srcs = [":package-srcs"],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
