#!/bin/bash
# API Testing Examples for CFD/FEA Platform

API_URL="http://localhost:8080/api/v1"

echo "=== CFD/FEA Platform API Tests ==="
echo

# 1. List all jobs
echo "1. Получение списка заданий:"
curl -s -X GET "${API_URL}/jobs" | jq '.'
echo
echo

# 2. Create CFD job
echo "2. Создание CFD задания:"
echo "   (требует файл test-cfd.tar.gz)"
# curl -X POST -F "type=cfd" -F "input=@test-cfd.tar.gz" "${API_URL}/jobs"
echo "   Раскомментируйте строку выше и укажите реальный файл"
echo
echo

# 3. Create FEA job
echo "3. Создание FEA задания:"
echo "   (требует файл test-fea.inp)"
# curl -X POST -F "type=fea" -F "input=@test-fea.inp" "${API_URL}/jobs"
echo "   Раскомментируйте строку выше и укажите реальный файл"
echo
echo

# 4. Get specific job
echo "4. Получение информации о задании:"
echo "   curl -X GET '${API_URL}/jobs?id=<job-id>'"
echo
echo

# 5. Download results
echo "5. Скачивание результатов:"
echo "   curl -X GET '${API_URL}/results?id=<job-id>' -o results.tar.gz"
echo
echo

# Example full workflow
echo "=== Пример полного цикла ==="
echo
echo "# Создать задание"
echo 'JOB_ID=$(curl -s -X POST -F "type=cfd" -F "input=@case.tar.gz" "${API_URL}/jobs" | jq -r ".id")'
echo
echo "# Проверить статус"
echo 'curl -s "${API_URL}/jobs?id=${JOB_ID}" | jq "."'
echo
echo "# Скачать результаты (после завершения)"
echo 'curl -X GET "${API_URL}/results?id=${JOB_ID}" -o results.tar.gz'
