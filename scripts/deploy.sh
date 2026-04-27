#!/bin/bash
# Rolling deploy — updates the binary on all running ASG instances via SSM Run Command.
# No instance replacement, no downtime. Takes ~10 seconds per instance.
#
# Usage:
#   ./scripts/deploy.sh                        # deploys latest
#   ./scripts/deploy.sh <git-sha>              # deploys a specific build
#
# Requirements:
#   - AWS CLI configured
#   - ssm:SendCommand permission on your IAM user/role
#   - Instances must have AmazonSSMManagedInstanceCore policy attached

set -euo pipefail

AWS_REGION="us-east-1"
DEPLOY_BUCKET="your-deploy-bucket"
ASG_NAME="ingestion-asg"
SHA="${1:-latest}"
BINARY_KEY="ingestion-server/${SHA}/ingestion-server"

echo "Deploying ingestion-server @ ${SHA} to ASG: ${ASG_NAME}"

COMMAND_ID=$(aws ssm send-command \
  --region "${AWS_REGION}" \
  --document-name "AWS-RunShellScript" \
  --targets "Key=tag:aws:autoscaling:groupName,Values=${ASG_NAME}" \
  --parameters "commands=[
    'aws s3 cp s3://${DEPLOY_BUCKET}/${BINARY_KEY} /usr/local/bin/ingestion-server --region ${AWS_REGION}',
    'chmod +x /usr/local/bin/ingestion-server',
    'systemctl restart ingestion-server',
    'sleep 3',
    'systemctl is-active ingestion-server && echo OK || (echo FAILED && exit 1)'
  ]" \
  --timeout-seconds 60 \
  --query "Command.CommandId" \
  --output text)

echo "SSM Command ID: ${COMMAND_ID}"
echo "Waiting for command to complete on all instances..."

aws ssm wait command-executed \
  --region "${AWS_REGION}" \
  --command-id "${COMMAND_ID}" \
  --instance-id "$(aws autoscaling describe-auto-scaling-groups \
    --auto-scaling-group-names "${ASG_NAME}" \
    --query "AutoScalingGroups[0].Instances[0].InstanceId" \
    --output text)"

# Print result per instance
aws ssm list-command-invocations \
  --region "${AWS_REGION}" \
  --command-id "${COMMAND_ID}" \
  --details \
  --query "CommandInvocations[].{Instance:InstanceId,Status:Status,Output:CommandPlugins[0].Output}" \
  --output table

echo "Deploy complete."
