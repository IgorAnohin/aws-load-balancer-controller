## How to install

Based on: https://docs.aws.amazon.com/eks/latest/userguide/lbc-manifest.html

0) Поднять 2 Worker Node

1) Prepare
```shell
$ echo 'export KUBECONFIG="/root/.kube/config"' >> ~/.bashrc
$ source ~/.bashrc
$ kubectl proxy &
$ kubectl apply \
    --validate=false \
    -f https://github.com/jetstack/cert-manager/releases/download/v1.13.5/cert-manager.yaml
```

2) Install NLB Provider:
```shell
$ curl -Lo v2_7_1_full.yaml https://github.com/kubernetes-sigs/aws-load-balancer-controller/releases/download/v2.7.1/v2_7_1_full.yaml
$
$ # Replace "ianokhin-nlb-provider" with your cluster name
$ sed -i.bak -e 's|your-cluster-name|ianokhin-nlb-provider|' ./v2_7_1_full.yaml
$ # Replace the following configs
      containers:
      - args:
        - --cluster-name=ianokhin-nlb-provider
        - --ingress-class=alb
>         - --enable-shield=false
>         - --aws-api-endpoints=ec2=https://api.cloud.croc.ru:443,elasticloadbalancing=https://elb.cloud.croc.ru:443
>         - --aws-vpc-id=vpc-C279FC93
>         - --aws-region=croc
>         - --enable-backend-security-group=false
>         - --feature-gates=NLBSecurityGroup=false,WeightedTargetGroups=false,ListenerRulesTagging=false

        image: public.ecr.aws/eks/aws-load-balancer-controller:v2.7.1
>         image: igranokhin/custom-aws-load-balancer-controller:debug.26
>         env:
>           - name: AWS_ACCESS_KEY_ID
>             value: "ianokhin:ianokhin@c2dev"
>           - name: AWS_SECRET_ACCESS_KEY
>             value: "XXXXXXXXXXXXXXXXXXXXXX"
```

3) Patch nodes, because we don't have ProviderID
```shell
$ # You must add Provider ID in format "croc/i-<instance-id>" for each node (Case is important!)
$ # Example:
$ # kubectl patch node ianokhin-test-master-40643a82 -p '{"spec":{"providerID":"croc/i-40643A82"}}'
$ # kubectl patch node ianokhin-test-ingress-8a801dc2 -p '{"spec":{"providerID":"croc/i-8A801DC2"}}'
$ # The following command generates required to run commands
$ kubectl get nodes | grep Ready | awk '{print $1}' | awk -F'-' '{print "kubectl patch node " $0 " -p \047{\"spec\":{\"providerID\":\"croc/i-" toupper($NF)  "\"}}\047" }'
```

4) Tag K8S security group to fix the following error
```shell
$ # To Fix:
$ # Error: expected exactly one securityGroup tagged with kubernetes.io/cluster/<cluster-name> for eni eni-08F486E2,
$ # Solution: Manually add empty tag "kubernetes.io/cluster/<cluster-name>" to Secutiry Group, created for K8S
```

5) Run NLB Provider
```shell
$ kubectl apply -f v2_7_1_full.yaml
$
$ $ To check NLB provider pod:
$ kubectl get pods --namespace kube-system | \
   grep aws-load | \
     awk '{print $1}' | \
       xargs -I{} kubectl logs --namespace kube-system -f {}
```

6) Deploy Nginx + Load Balancer Service:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-webserver
  annotations:
    service.beta.kubernetes.io/aws-load-balancer-scheme: "internet-facing"
spec:
  selector:
    app: web
  ports:
    - protocol: TCP
      port: 80
      targetPort: 80
  type: LoadBalancer
---
apiVersion: v1
kind: Pod           
                    
metadata:
  name: webserver
  labels:
    app: web
spec:
  containers:
  - name: webserver
    image: nginx:latest
    ports:
    - containerPort: 80
```
