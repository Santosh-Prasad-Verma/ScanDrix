FROM rabbitmq:3.13-management-alpine

# Copy and enable the RabbitMQ delayed message exchange plugin
COPY rabbitmq_delayed_message_exchange-3.13.0.ez /plugins/
RUN rabbitmq-plugins enable --offline rabbitmq_delayed_message_exchange rabbitmq_management rabbitmq_prometheus

COPY rabbitmq.conf /etc/rabbitmq/rabbitmq.conf
