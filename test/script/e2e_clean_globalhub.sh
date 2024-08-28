wait_cmd "kubectl delete mgh --all -n multicluster-global-hub"

## wait kafka/kafkatopic/kafka user be deleted
sleep 10

if [[ -z $(kubectl get kafkatopic "$cluster" --context "$hub" --ignore-not-found) ]]; then
  echo "Failed to delete kafkatopics"
  return 1
fi
if [[ -z $(kubectl get kafkauser "$cluster" --context "$hub" --ignore-not-found) ]]; then
  echo "Failed to delete kafkausers"
  return 1
fi
if [[ -z $(kubectl get kafka "$cluster" --context "$hub" --ignore-not-found) ]]; then
  echo "Failed to delete kafka"
  return 1
fi
