import defradb
from defradb import DefraConfig, DefraClient
import random
from datetime import datetime, timedelta

schema = """
type Project {
    name: String
    handle: String
    assessments: [Assessment]
}

type Assessment {
    project: Project
    date: DateTime
    latent_variables: [LatentVariable]
}

type LatentVariable {
    assessment: Assessment
    badge: Badge
    name: String
    domain: String
    categories: [String]
    ordered_categorical: Boolean
    timeseries_real: [LatentTimestampValueReal]
    timeseries_categorical: [LatentTimestampValueCategorical]
    indicators: [Indicator]
}

type LatentTimestampValueReal {
    variable: LatentVariable
    timestamp: DateTime
    upper_ci95: Float
    lower_ci95: Float
    median: Float
    sigmoid_negentropy: Float
}

type ObservableTimestampValueReal {
    variable: Observable
    timestamp: DateTime
    upper_ci95: Float
    lower_ci95: Float
    median: Float
    sigmoid_negentropy: Float
}

type LatentTimestampValueCategorical {
    variable: LatentVariable
    timestamp: DateTime
    mode: Int
    sigmoid_negentropy: Float
}

type ObservableTimestampValueCategorical {
    variable: Observable
    timestamp: DateTime
    mode: Int
    sigmoid_negentropy: Float
}

type Indicator {
    latent_variable: LatentVariable
    observable: Observable
    correlation: Float
    mutual_information: Float
}

type Observable {
    indicates: [Indicator]
    domain: String
    name: String
    timeseries_real: [ObservableTimestampValueReal]
    timeseries_categorical: [ObservableTimestampValueCategorical]
    method: Method
    evidence: [Evidence]
}

type Evidence {
    observable: Observable
    uploaded_by: User
    name: String
    asset_url: String
    confidence: Float
    uploaded: DateTime
    location: String
}

type User {
    evidence: [Evidence]
    name: String
    profile_url: String
}

type Method {
    observable: Observable
    name: String
}

type Badge {
    variable: LatentVariable
    name: String
    handle: String
    description: String
    unit: String
    more_is_better: Boolean
    time_unit: String
    badge_threshold: Float
    zero_threshold: Float
    confidence: Float
}
"""

# Create the defradb configuration.
config = DefraConfig(
    api_url="localhost:9181/api/v0/",
    tcp_multiaddr="localhost:9161",
)

# Create the defradb client.
client = DefraClient(config)

def random_datetime():
  """
  Generate a random RFC 3339 formatted datetime
  """
  random_seconds = random.randint(0, 10**6)
  random_dt = datetime.utcnow() - timedelta(seconds=random_seconds)
  return random_dt.isoformat() + "Z"

def rand_project():
 return {
    "name": "Project 593",
    "handle": "project-8"
  }

def rand_assessment(project_key):
  return {
    "project_id": project_key,  # Assumes that you've already created the project and have its key
    "date": random_datetime()
  }

def rand_latent_variable(assessment_key, domain):
  return {
    "assessment_id": assessment_key,  # Assumes that you've already created the assessment and have its key
    "name": "Variable " + str(random.randint(1, 10000)),
    "domain": domain,
    "categories": ["Dead", "Alive", "Thriving"],
    "ordered_categorical": True
  }

def rand_timestampvalue_real(variable_key):
  median = random.uniform(0, 10000)
  return {
    "variable_id": variable_key,  # Assumes that you've already created the variable and have its key
    "timestamp": random_datetime(),
    "upper_ci95": median - random.uniform(0, 500),
    "lower_ci95": median + random.uniform(0, 500),
    "median": median,
    "sigmoid_negentropy": random.uniform(0, 1)
}

def rand_timestampvalue_categorical(variable_key):
  return {
    "variable_id": variable_key, # Assumes that you've already created the variable and have its key
    "timestamp": random_datetime(),
    "mode": random.randint(1, 3),
    "sigmoid_negentropy": random.uniform(0, 1)
  }

def rand_observable(domain):
  return {
    "domain": domain,
    "name": "Observable " + str(random.randint(1, 1000)),
  }

def rand_indicator(variable_key, observable_key):
  return {
    "observable_id": observable_key,  # Assumes that you've already created the observable and have its key
    "latent_variable_id": variable_key, # Assumes that you've already created the variable and have its key
    "correlation": random.uniform(0, 1),
    "mutual_information": random.uniform(0, 1)
  }

def rand_evidence(observable_key, user_key):
  return {
    "observable_id": observable_key, # Assumes that you've already created the observable and have its key
    "uploaded_by_id": user_key,
    "name": "Evidence " + str(random.randint(1, 1000)),
    "asset_url": "https://example.com/evidence/" + str(random.randint(1, 1000)),
    "confidence": random.uniform(0, 1),
    "uploaded": random_datetime(),
  }

def rand_user():
  return {
    "name": "User " + str(random.randint(1, 3)),
    "profile_url": "https://example.com/profile/" + str(random.randint(1, 1000))
  }

def rand_method(observable_key):
  return {
    "observable_id": observable_key,
    "name": random.choice(["satellite", "expert_attestation", "iot_sensor", "image"])
  }

def rand_badge(variable_key):
  return {
    "variable_id": variable_key, # Assumes that you've already created the variable and have its key
    "handle": "badge-" + str(random.randint(1, 1000)),
    "name": "Badge " + str(random.randint(1, 1000)),
    "description": "Description of Badge " + str(random.randint(1, 1000)),
    "unit": "Unit " + str(random.randint(1, 1000)),
    "more_is_better": random.choice([True, False]),
    "time_unit": "Time Unit " + str(random.randint(1, 1000)),
    "badge_threshold": random.uniform(0, 3),
    "zero_threshold":  0,
    "confidence": random.uniform(0, 1)
  }

typenames = {
    "Project": rand_project,
    "Assessment": rand_assessment,
    "LatentVariable": rand_latent_variable,
    "LatentTimestampValueReal": rand_timestampvalue_real,
    "LatentTimestampValueCategorical": rand_timestampvalue_categorical,
    "ObservableTimestampValueReal": rand_timestampvalue_real,
    "ObservableTimestampValueCategorical": rand_timestampvalue_categorical,
    "Indicator": rand_indicator,
    "Observable": rand_observable,
    "Evidence": rand_evidence,
    "User": rand_user,
    "Method": rand_method,
    "Badge": rand_badge,
  }

def create(typename, *args, key_response=False):
  """
  Create a random defradb entry for this typename
  typename: the typename to create (e.g. Project)
  keys: the key arguments to each of those generation functions
  """
  request = defradb.dict_to_create_query(typename, typenames[typename](*args))
  response = client.request(request)

  if not key_response:
    # get the key of the new entry and return it
    return response[0]["_key"]
  else:
    return response[0]["_key"], response[0]

if True:
  client.load_schema(schema)

def rand_timeseries(key, domain, kind):
  """
  Build a categorical and a real timeseries for the given key
  key: the key
  kind: one of Observable or LatentVariable
  """
  match kind:
    case "Observable":
      if domain == "Real":
        timestampvaluereal_key, timestampreal = create("ObservableTimestampValueReal", key, key_response=True)
      elif domain == "Categorical":
        timestampvalueobs_key, timestampobs = create("ObservableTimestampValueCategorical", key, key_response=True)
    case "LatentVariable":
      if domain == "Real":  
        timestampvaluereal_key, timestampreal = create("LatentTimestampValueReal", key, key_response=True)
      elif domain == "Categorical":
        timestampvalueobs_key, timestampobs = create("LatentTimestampValueCategorical", key, key_response=True)


projk = create("Project")
for _ in range(2):
  assessment_key = create("Assessment", projk)

  for _ in range(2):
    # variable_domain = random.choice(["Real", "Categorical"])
    variable_domain = random.choice(["Real"])
    variable_key = create("LatentVariable", assessment_key, variable_domain)

    create("Badge", variable_key)

    # build the timeseries for the latent variable
    for _ in range(3):
      rand_timeseries(variable_key, variable_domain, "LatentVariable")

    # create the indicators
    for _ in range(3):
      # observable_domain = random.choice(["Real", "Categorical"])
      observable_domain = random.choice(["Real"])

      # one observable for each indicator
      observable_key, observable = create("Observable", observable_domain, key_response=True)

      indicator_key = create("Indicator", variable_key, observable_key)

      # build the timeseries for the observable
      for _ in range(3):
        rand_timeseries(observable_key, observable_domain, "Observable")

      create("Method", observable_key)
    
      # create evidences on the observable
      for _ in range(random.randint(1,3)):
        user_key = create("User")
        evidence_key = create("Evidence", observable_key, user_key)

