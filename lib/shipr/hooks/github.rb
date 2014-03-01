module Shipr
  module Hooks
    class GitHub < Grape::API
      logger Shipr.logger
      format :json
      default_format :json

      # Payload is a string, so we need to parse it.
      parser :json, -> (object, env) {
        hash = MultiJson.load(object)
        if payload = hash['payload']
          hash['payload'] = MultiJson.load(payload)
        end
        hash
      }

      helpers do
        def event
          headers['X-Github-Event']
        end

        def ping?
          event == 'ping'
        end

        def deployment?
          event == 'deployment'
        end

        def deploy
          deployment? ? GitHubJobCreator.create(params) : {}
        end
      end

      helpers do
        delegate :pusher, to: :'Shipr'
        delegate :authenticated?, to: :warden

        def warden; env['warden'] end
      end

      use Warden::Manager

      params do
        requires :id,
          type: Integer,
          desc: 'The deployment id.'
        requires :sha,
          type: String,
          desc: 'The sha to deploy.'
        requires :name,
          type: String,
          desc: 'The repo to deploy (<user>/<repo>).'
        optional :description,
          type: String,
          desc: 'The description of the deploy.'
        group :payload do
          optional :environment,
            type: String,
            desc: 'The environment to deploy to.'
        end
      end
      post do
        if authenticated?
          status 200
          present deploy
        else
          error!('Forbidden', 403)
        end
      end
    end
  end
end
