package aws

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"golang.org/x/crypto/ssh"
)

type Instance struct {
	ID       string
	PublicIP string
	SGId     string
	KeyName  string
}

type StepFunc func(msg string)

// Launch creates an ephemeral EC2 instance with a generated ed25519 key pair
// and a dedicated security group. Returns the instance and the private key PEM
// to be used for the SSH tunnel.
func Launch(ctx context.Context, region, instanceType string, step StepFunc) (*Instance, []byte, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, nil, fmt.Errorf("load aws config: %w", err)
	}
	svc := ec2.NewFromConfig(cfg)

	step("Creating security group")
	sgID, err := createSecurityGroup(ctx, svc)
	if err != nil {
		return nil, nil, fmt.Errorf("security group: %w", err)
	}

	step("Resolving latest Amazon Linux 2 AMI")
	amiID, err := latestAmazonLinux2AMI(ctx, svc, archFromInstanceType(instanceType))
	if err != nil {
		step("Deleting security group")
		_ = deleteSecurityGroup(context.Background(), svc, sgID)
		return nil, nil, fmt.Errorf("ami lookup: %w", err)
	}

	step("Generating SSH key pair")
	keyName, privKeyPEM, err := generateAndImportKey(ctx, svc)
	if err != nil {
		step("Deleting security group")
		_ = deleteSecurityGroup(context.Background(), svc, sgID)
		return nil, nil, fmt.Errorf("key pair: %w", err)
	}

	step("Launching instance")
	instanceID, err := runInstance(ctx, svc, amiID, instanceType, keyName, sgID)
	if err != nil {
		step("Deleting key pair")
		_ = deleteKeyPair(context.Background(), svc, keyName)
		step("Deleting security group")
		_ = deleteSecurityGroup(context.Background(), svc, sgID)
		return nil, nil, fmt.Errorf("run instance: %w", err)
	}

	step("Waiting for instance to be running")
	publicIP, err := waitRunning(ctx, svc, instanceID)
	if err != nil {
		step("Terminating instance")
		_ = terminateInstance(context.Background(), svc, instanceID)
		step("Deleting key pair")
		_ = deleteKeyPair(context.Background(), svc, keyName)
		step("Deleting security group")
		_ = deleteSecurityGroup(context.Background(), svc, sgID)
		return nil, nil, fmt.Errorf("wait running: %w", err)
	}

	return &Instance{
		ID:       instanceID,
		PublicIP: publicIP,
		SGId:     sgID,
		KeyName:  keyName,
	}, privKeyPEM, nil
}

// Terminate stops the instance and removes the key pair and security group.
func Terminate(ctx context.Context, region string, inst *Instance, step StepFunc) error {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return fmt.Errorf("load aws config: %w", err)
	}
	svc := ec2.NewFromConfig(cfg)

	step("Terminating instance")
	if err := terminateInstance(ctx, svc, inst.ID); err != nil {
		return err
	}

	step("Waiting for instance to terminate")
	if err := waitTerminated(ctx, svc, inst.ID); err != nil {
		return err
	}

	step("Deleting key pair")
	_ = deleteKeyPair(ctx, svc, inst.KeyName)

	step("Deleting security group")
	return deleteSecurityGroup(ctx, svc, inst.SGId)
}

func generateAndImportKey(ctx context.Context, svc *ec2.Client) (name string, privKeyPEM []byte, err error) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", nil, fmt.Errorf("generate key: %w", err)
	}

	sshPubKey, err := ssh.NewPublicKey(pubKey)
	if err != nil {
		return "", nil, fmt.Errorf("marshal public key: %w", err)
	}

	block, err := ssh.MarshalPrivateKey(privKey, "")
	if err != nil {
		return "", nil, fmt.Errorf("marshal private key: %w", err)
	}
	privKeyPEM = pem.EncodeToMemory(block)

	name = fmt.Sprintf("navsat-%d", time.Now().Unix())
	_, err = svc.ImportKeyPair(ctx, &ec2.ImportKeyPairInput{
		KeyName:           aws.String(name),
		PublicKeyMaterial: ssh.MarshalAuthorizedKey(sshPubKey),
	})
	if err != nil {
		return "", nil, fmt.Errorf("import key pair: %w", err)
	}

	return name, privKeyPEM, nil
}

func deleteKeyPair(ctx context.Context, svc *ec2.Client, keyName string) error {
	_, err := svc.DeleteKeyPair(ctx, &ec2.DeleteKeyPairInput{
		KeyName: aws.String(keyName),
	})
	return err
}

func createSecurityGroup(ctx context.Context, svc *ec2.Client) (string, error) {
	name := fmt.Sprintf("navsat-%d", time.Now().Unix())
	out, err := svc.CreateSecurityGroup(ctx, &ec2.CreateSecurityGroupInput{
		GroupName:   aws.String(name),
		Description: aws.String("navsat ephemeral SSH proxy"),
	})
	if err != nil {
		return "", err
	}
	sgID := aws.ToString(out.GroupId)

	_, err = svc.AuthorizeSecurityGroupIngress(ctx, &ec2.AuthorizeSecurityGroupIngressInput{
		GroupId: aws.String(sgID),
		IpPermissions: []types.IpPermission{
			{
				IpProtocol: aws.String("tcp"),
				FromPort:   aws.Int32(22),
				ToPort:     aws.Int32(22),
				IpRanges:   []types.IpRange{{CidrIp: aws.String("0.0.0.0/0")}},
			},
		},
	})
	return sgID, err
}

func deleteSecurityGroup(ctx context.Context, svc *ec2.Client, sgID string) error {
	_, err := svc.DeleteSecurityGroup(ctx, &ec2.DeleteSecurityGroupInput{
		GroupId: aws.String(sgID),
	})
	return err
}

func archFromInstanceType(instanceType string) string {
	family := strings.SplitN(instanceType, ".", 2)[0]
	if strings.HasSuffix(family, "g") || family == "a1" {
		return "arm64"
	}
	return "x86_64"
}

func latestAmazonLinux2AMI(ctx context.Context, svc *ec2.Client, arch string) (string, error) {
	out, err := svc.DescribeImages(ctx, &ec2.DescribeImagesInput{
		Owners: []string{"amazon"},
		Filters: []types.Filter{
			{Name: aws.String("name"), Values: []string{"amzn2-ami-hvm-*-" + arch + "-gp2"}},
			{Name: aws.String("state"), Values: []string{"available"}},
		},
	})
	if err != nil {
		return "", err
	}
	if len(out.Images) == 0 {
		return "", fmt.Errorf("no Amazon Linux 2 AMI found")
	}
	latest := out.Images[0]
	for _, img := range out.Images[1:] {
		if aws.ToString(img.CreationDate) > aws.ToString(latest.CreationDate) {
			latest = img
		}
	}
	return aws.ToString(latest.ImageId), nil
}

func runInstance(ctx context.Context, svc *ec2.Client, amiID, instanceType, keyName, sgID string) (string, error) {
	out, err := svc.RunInstances(ctx, &ec2.RunInstancesInput{
		ImageId:          aws.String(amiID),
		InstanceType:     types.InstanceType(instanceType),
		MinCount:         aws.Int32(1),
		MaxCount:         aws.Int32(1),
		KeyName:          aws.String(keyName),
		SecurityGroupIds: []string{sgID},
		TagSpecifications: []types.TagSpecification{
			{
				ResourceType: types.ResourceTypeInstance,
				Tags: []types.Tag{
					{Key: aws.String("Name"), Value: aws.String("navsat-proxy")},
					{Key: aws.String("ManagedBy"), Value: aws.String("navsat")},
				},
			},
		},
	})
	if err != nil {
		return "", err
	}
	if len(out.Instances) == 0 {
		return "", fmt.Errorf("no instance returned")
	}
	return aws.ToString(out.Instances[0].InstanceId), nil
}

func waitRunning(ctx context.Context, svc *ec2.Client, instanceID string) (string, error) {
	waiter := ec2.NewInstanceRunningWaiter(svc)
	if err := waiter.Wait(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []string{instanceID},
	}, 3*time.Minute); err != nil {
		return "", err
	}

	out, err := svc.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []string{instanceID},
	})
	if err != nil {
		return "", err
	}
	for _, r := range out.Reservations {
		for _, i := range r.Instances {
			if aws.ToString(i.PublicIpAddress) != "" {
				return aws.ToString(i.PublicIpAddress), nil
			}
		}
	}
	return "", fmt.Errorf("instance has no public IP")
}

func terminateInstance(ctx context.Context, svc *ec2.Client, instanceID string) error {
	_, err := svc.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
		InstanceIds: []string{instanceID},
	})
	return err
}

func waitTerminated(ctx context.Context, svc *ec2.Client, instanceID string) error {
	waiter := ec2.NewInstanceTerminatedWaiter(svc)
	return waiter.Wait(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []string{instanceID},
	}, 3*time.Minute)
}
