module Shipr
  module Notifiers
    class Slack < Base
      MESSAGES = {
        pending: ['#ff0', "Deploying %s@%s to %s"],
        success: ['#0f0', "Deployed %s@%s to %s"],
        failure: ['#f00', "Failed to deploy %s@%s to %s"],
        error:   ['#f00', "Failed to deploy %s@%s to %s"]
      }

      def notify
        HTTParty.post "https://#{account}.slack.com/services/hooks/incoming-webhook?token=#{token}",
          body: "payload=#{JSON.dump(attachments: [attachment])}"
      end

    private

      def attachment
        color, template = MESSAGES[state.to_sym]
        message = template % [payload.name, payload.sha, payload.payload.environment]
        { fallback: message, color: color, fields: [{ value: message }] }
      end

      def account
        ENV['SLACK_ACCOUNT']
      end

      def token
        ENV['SLACK_TOKEN']
      end
    end
  end
end