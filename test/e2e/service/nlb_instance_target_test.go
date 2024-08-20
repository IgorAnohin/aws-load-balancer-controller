package service

import (
	"context"
	"strings"

	awssdk "github.com/aws/aws-sdk-go/aws"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
)

var _ = Describe("test k8s service reconciled by the aws load balancer controller", func() {
	var (
		ctx     context.Context
		stack   NLBInstanceTestStack
		dnsName string
		lbARN   string
	)
	BeforeEach(func() {
		ctx = context.Background()
		stack = NLBInstanceTestStack{}
	})
	AfterEach(func() {
		err := stack.Cleanup(ctx, tf)
		Expect(err).NotTo(HaveOccurred())
	})
	Context("with NLB instance target configuration", func() {
		annotation := make(map[string]string)
		BeforeEach(func() {
			if tf.Options.IPFamily == "IPv6" {
				annotation["service.beta.kubernetes.io/aws-load-balancer-ip-address-type"] = "dualstack"
			}
		})
		It("should provision internet-facing load balancer resources", func() {
			annotation["service.beta.kubernetes.io/aws-load-balancer-scheme"] = "internet-facing"
			By("deploying stack", func() {
				err := stack.Deploy(ctx, tf, annotation)
				Expect(err).NotTo(HaveOccurred())
			})

			By("checking service status for lb dns name", func() {
				dnsName = stack.GetLoadBalancerIngressHostName()
				Expect(dnsName).ToNot(BeEmpty())
			})

			By("querying AWS loadbalancer from the dns name", func() {
				var err error
				lbARN, err = tf.LBManager.FindLoadBalancerByDNSName(ctx, dnsName)
				Expect(err).NotTo(HaveOccurred())
				Expect(lbARN).ToNot(BeEmpty())
			})

			By("verifying AWS loadbalancer resources", func() {
				nodeList, err := stack.GetWorkerNodes(ctx, tf)
				Expect(err).ToNot(HaveOccurred())
				err = verifyAWSLoadBalancerResources(ctx, tf, lbARN, LoadBalancerExpectation{
					Type:         "network",
					Scheme:       "internet-facing",
					TargetType:   "instance",
					Listeners:    stack.resourceStack.getListenersPortMap(),
					TargetGroups: stack.resourceStack.getTargetGroupNodePortMap(),
					NumTargets:   len(nodeList),
					TargetGroupHC: &TargetGroupHC{
						Protocol:           "TCP",
						Port:               "traffic-port",
						Interval:           10,
						Timeout:            10,
						HealthyThreshold:   3,
						UnhealthyThreshold: 3,
					},
				})
				Expect(err).NotTo(HaveOccurred())
			})
			By("waiting for target group targets to be healthy", func() {
				nodeList, err := stack.GetWorkerNodes(ctx, tf)
				Expect(err).ToNot(HaveOccurred())
				err = waitUntilTargetsAreHealthy(ctx, tf, lbARN, len(nodeList))
				Expect(err).NotTo(HaveOccurred())
			})
			By("waiting until DNS name is available", func() {
				err := utils.WaitUntilDNSNameAvailable(ctx, dnsName)
				Expect(err).NotTo(HaveOccurred())
			})
		})
		It("should provision internal load-balancer resources", func() {
			By("deploying stack", func() {
				annotation["service.beta.kubernetes.io/aws-load-balancer-scheme"] = "internal"
				err := stack.Deploy(ctx, tf, annotation)
				Expect(err).NotTo(HaveOccurred())
			})
			By("checking service status for lb dns name", func() {
				dnsName = stack.GetLoadBalancerIngressHostName()
				Expect(dnsName).ToNot(BeEmpty())
			})

			By("querying AWS loadbalancer from the dns name", func() {
				var err error
				lbARN, err = tf.LBManager.FindLoadBalancerByDNSName(ctx, dnsName)
				Expect(err).NotTo(HaveOccurred())
				Expect(lbARN).ToNot(BeEmpty())
			})
			By("verifying AWS loadbalancer resources", func() {
				nodeList, err := stack.GetWorkerNodes(ctx, tf)
				Expect(err).ToNot(HaveOccurred())
				err = verifyAWSLoadBalancerResources(ctx, tf, lbARN, LoadBalancerExpectation{
					Type:         "network",
					Scheme:       "internal",
					TargetType:   "instance",
					Listeners:    stack.resourceStack.getListenersPortMap(),
					TargetGroups: stack.resourceStack.getTargetGroupNodePortMap(),
					NumTargets:   len(nodeList),
					TargetGroupHC: &TargetGroupHC{
						Protocol:           "TCP",
						Port:               "traffic-port",
						Interval:           10,
						Timeout:            10,
						HealthyThreshold:   3,
						UnhealthyThreshold: 3,
					},
				})
				Expect(err).NotTo(HaveOccurred())
			})
			By("specifying target group attributes annotation", func() {
				err := stack.UpdateServiceAnnotations(ctx, tf, map[string]string{
					"service.beta.kubernetes.io/aws-load-balancer-target-group-attributes": "preserve_client_ip.enabled=false, proxy_protocol_v2.enabled=true, deregistration_delay.timeout_seconds=120",
				})
				Expect(err).NotTo(HaveOccurred())

				Eventually(func() bool {
					return verifyTargetGroupAttributes(ctx, tf, lbARN, map[string]string{
						"preserve_client_ip.enabled":           "false",
						"proxy_protocol_v2.enabled":            "true",
						"deregistration_delay.timeout_seconds": "120",
					})
				}, utils.PollTimeoutShort, utils.PollIntervalMedium).Should(BeTrue())
			})
			By("waiting for load balancer to be available", func() {
				err := tf.LBManager.WaitUntilLoadBalancerAvailable(ctx, lbARN)
				Expect(err).NotTo(HaveOccurred())
			})
		})
		It("should create TLS listeners", func() {
			if len(tf.Options.CertificateARNs) == 0 {
				Skip("Skipping tests, certificates not specified")
			}
			By("deploying stack", func() {
				annotation["service.beta.kubernetes.io/aws-load-balancer-ssl-cert"] = tf.Options.CertificateARNs
				annotation["service.beta.kubernetes.io/aws-load-balancer-scheme"] = "internet-facing"
				err := stack.Deploy(ctx, tf, annotation)
				Expect(err).NotTo(HaveOccurred())
			})
			By("checking service status for lb dns name", func() {
				dnsName = stack.GetLoadBalancerIngressHostName()
				Expect(dnsName).ToNot(BeEmpty())
			})

			By("querying AWS loadbalancer from the dns name", func() {
				var err error
				lbARN, err = tf.LBManager.FindLoadBalancerByDNSName(ctx, dnsName)
				Expect(err).NotTo(HaveOccurred())
				Expect(lbARN).ToNot(BeEmpty())
			})
			By("verifying AWS loadbalancer resources", func() {
				err := verifyAWSLoadBalancerResources(ctx, tf, lbARN, LoadBalancerExpectation{
					Type:       "network",
					Scheme:     "internet-facing",
					TargetType: "instance",
					Listeners: map[string]string{
						"80": "TLS",
					},
					TargetGroups: stack.resourceStack.getTargetGroupNodePortMap(),
					NumTargets:   0,
					TargetGroupHC: &TargetGroupHC{
						Protocol:           "TCP",
						Port:               "traffic-port",
						Interval:           10,
						Timeout:            10,
						HealthyThreshold:   3,
						UnhealthyThreshold: 3,
					},
				})
				Expect(err).NotTo(HaveOccurred())
			})
			By("verifying listener certificates", func() {
				expectedARNs := strings.Split(tf.Options.CertificateARNs, ",")
				Eventually(func() bool {
					return verifyLoadBalancerListenerCertificates(ctx, tf, lbARN, expectedARNs) == nil
				}, utils.PollTimeoutShort, utils.PollIntervalMedium).Should(BeTrue())
			})
			By("removing first certificate from annotation and updating the service", func() {
				certs := strings.Split(tf.Options.CertificateARNs, ",")[1:]
				if len(certs) == 0 {
					return
				}
				err := stack.UpdateServiceAnnotations(ctx, tf, map[string]string{
					"service.beta.kubernetes.io/aws-load-balancer-ssl-cert": strings.Join(certs, ","),
				})
				Expect(err).NotTo(HaveOccurred())
				Eventually(func() bool {
					return verifyLoadBalancerListenerCertificates(ctx, tf, lbARN, certs) == nil
				}, utils.PollTimeoutShort, utils.PollIntervalMedium).Should(BeTrue())
			})
			By("waiting for load balancer to be available", func() {
				err := tf.LBManager.WaitUntilLoadBalancerAvailable(ctx, lbARN)
				Expect(err).NotTo(HaveOccurred())
			})
		})
		It("should enable proxy protocol v2", func() {
			By("deploying stack", func() {
				annotation["service.beta.kubernetes.io/aws-load-balancer-proxy-protocol"] = "*"
				err := stack.Deploy(ctx, tf, annotation)
				Expect(err).ToNot(HaveOccurred())
				dnsName = stack.GetLoadBalancerIngressHostName()
				Expect(dnsName).ToNot(BeEmpty())
				lbARN, err = tf.LBManager.FindLoadBalancerByDNSName(ctx, dnsName)
				Expect(err).NotTo(HaveOccurred())
				Expect(lbARN).ToNot(BeEmpty())
			})
			By("verifying target group attributes", func() {
				verified := verifyTargetGroupAttributes(ctx, tf, lbARN, map[string]string{
					"proxy_protocol_v2.enabled": "true",
				})
				Expect(verified).To(BeTrue())
			})
			By("verifying precedence with target group attributes configuration", func() {
				err := stack.UpdateServiceAnnotations(ctx, tf, map[string]string{
					"service.beta.kubernetes.io/aws-load-balancer-target-group-attributes": "proxy_protocol_v2.enabled=false, deregistration_delay.timeout_seconds=120",
				})
				Expect(err).NotTo(HaveOccurred())
				Eventually(func() bool {
					return verifyTargetGroupAttributes(ctx, tf, lbARN, map[string]string{
						"proxy_protocol_v2.enabled":            "true",
						"deregistration_delay.timeout_seconds": "120",
					})
				}, utils.PollTimeoutShort, utils.PollIntervalMedium).Should(BeTrue())
			})
			By("waiting for load balancer to be available", func() {
				err := tf.LBManager.WaitUntilLoadBalancerAvailable(ctx, lbARN)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Context("with NLB instance target configuration with target node labels", func() {
		annotation := make(map[string]string)
		BeforeEach(func() {
			if tf.Options.IPFamily == "IPv6" {
				annotation["service.beta.kubernetes.io/aws-load-balancer-ip-address-type"] = "dualstack"
			}
		})
		It("should add only the labelled nodes to the target group", func() {
			By("deploying stack", func() {
				annotation["service.beta.kubernetes.io/aws-load-balancer-target-node-labels"] = "service.node.label/key1=value1"
				err := stack.Deploy(ctx, tf, annotation)
				Expect(err).ToNot(HaveOccurred())
				dnsName = stack.GetLoadBalancerIngressHostName()
				Expect(dnsName).ToNot(BeEmpty())
				lbARN, err = tf.LBManager.FindLoadBalancerByDNSName(ctx, dnsName)
				Expect(err).NotTo(HaveOccurred())
				Expect(lbARN).ToNot(BeEmpty())
			})
			By("applying label to 1 worker node", func() {
				nodes, err := stack.GetWorkerNodes(ctx, tf)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(nodes)).To(BeNumerically(">", 0))
				err = stack.ApplyNodeLabels(ctx, tf, &nodes[0], map[string]string{"service.node.label/key1": "value1"})
				Expect(err).ToNot(HaveOccurred())

				targetGroups, err := tf.TGManager.GetTargetGroupsForLoadBalancer(ctx, lbARN)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(targetGroups)).To(Equal(1))
				tgARN := awssdk.StringValue(targetGroups[0].TargetGroupArn)

				err = verifyTargetGroupNumRegistered(ctx, tf, tgARN, 1)
				Expect(err).ToNot(HaveOccurred())
			})
			By("removing target-node-labels annotation from the service", func() {
				err := stack.DeleteServiceAnnotations(ctx, tf, []string{"service.beta.kubernetes.io/aws-load-balancer-target-node-labels"})
				Expect(err).ToNot(HaveOccurred())

				targetGroups, err := tf.TGManager.GetTargetGroupsForLoadBalancer(ctx, lbARN)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(targetGroups)).To(Equal(1))
				tgARN := awssdk.StringValue(targetGroups[0].TargetGroupArn)

				nodes, err := stack.GetWorkerNodes(ctx, tf)
				Expect(err).ToNot(HaveOccurred())

				err = verifyTargetGroupNumRegistered(ctx, tf, tgARN, len(nodes))
				Expect(err).ToNot(HaveOccurred())
			})
			By("waiting for load balancer to be available", func() {
				err := tf.LBManager.WaitUntilLoadBalancerAvailable(ctx, lbARN)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})
