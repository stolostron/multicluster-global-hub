# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

# Format the result to add Observability label for BeforeSuite and AfterSuite
# Log a requirement in ginkgo - https://github.com/onsi/ginkgo/issues/795

#!/bin/bash

if [ -z $1 ]; then
    echo "Please provide the results file."
    exit 1
fi

sed -i "s~BeforeSuite~Hoh: [P1][Sev1][hoh] Cannot enable hoh service successfully~g" $1
sed -i "s~AfterSuite~Hoh: [P1][Sev1][hoh] Cannot uninstall hoh service completely~g" $1
