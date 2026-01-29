package metrics

import (
	"sync"

	"github.com/openshift/multiarch-tuning-operator/pkg/utils"

	metrics2 "sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	TimeToProcessPod        prometheus.Histogram
	TimeToProcessGatedPod   prometheus.Histogram
	TimeToInspectImage      prometheus.Histogram
	TimeToInspectPodImages  prometheus.Histogram
	ProcessedPodsCtrl       prometheus.Counter
	FailedInspectionCounter prometheus.Counter
	PPCsProcessedPerPod     prometheus.Histogram
	ArchitecturesSkipped    prometheus.Counter
	PPCListingErrors        prometheus.Counter
	AnnotationSizeBytes     prometheus.Histogram
)

var onceController sync.Once

func InitPodPlacementControllerMetrics() {
	onceController.Do(initPodPlacementControllerMetrics)
}

func initPodPlacementControllerMetrics() {
	initCommonMetrics()
	TimeToProcessPod = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "mto_ppo_ctrl_time_to_process_pod_seconds",
			Help:    "Time taken to process any pod",
			Buckets: utils.Buckets(),
		},
	)
	TimeToProcessGatedPod = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "mto_ppo_ctrl_time_to_process_gated_pod_seconds",
			Help:    "Time taken to process a pod that is gated (includes inspection)",
			Buckets: utils.Buckets(),
		},
	)
	TimeToInspectImage = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "mto_ppo_ctrl_time_to_inspect_image_seconds",
			Help:    "Time taken to inspect an image",
			Buckets: utils.Buckets(),
		},
	)
	TimeToInspectPodImages = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "mto_ppo_ctrl_time_to_inspect_pod_images_seconds",
			Help:    "The time taken to inspect all the images in a pod (all containers)",
			Buckets: utils.Buckets(),
		},
	)
	ProcessedPodsCtrl = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "mto_ppo_ctrl_processed_pods_total",
			Help: "The total number of pods processed by the pod placement controller that had a scheduling gate",
		},
	)
	FailedInspectionCounter = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "mto_ppo_ctrl_failed_image_inspection_total",
			Help: "The total number of image inspections that failed",
		},
	)
	PPCsProcessedPerPod = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "mto_ppo_ctrl_ppcs_processed_per_pod",
			Help:    "Number of PodPlacementConfigs processed per pod",
			Buckets: []float64{0, 1, 2, 3, 5, 10},
		},
	)
	ArchitecturesSkipped = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "mto_ppo_ctrl_architectures_skipped_total",
			Help: "Total architectures skipped due to conflicts when applying PPC/CPPC preferred affinity",
		},
	)
	PPCListingErrors = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "mto_ppo_ctrl_ppc_listing_errors_total",
			Help: "Total errors when listing PodPlacementConfigs",
		},
	)
	AnnotationSizeBytes = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "mto_ppo_ctrl_preferred_affinity_annotation_size_bytes",
			Help:    "Size of the preferred-affinity-sources annotation in bytes",
			Buckets: prometheus.ExponentialBuckets(100, 2, 10), // 100B to 51.2KB
		},
	)
	metrics2.Registry.MustRegister(TimeToProcessPod, TimeToProcessGatedPod, TimeToInspectImage,
		TimeToInspectPodImages, ProcessedPodsCtrl, FailedInspectionCounter, PPCsProcessedPerPod,
		ArchitecturesSkipped, PPCListingErrors, AnnotationSizeBytes)
}
